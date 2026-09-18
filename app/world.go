package app

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"cposim/behavior"
	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/identifier"
	"cposim/gateway/metrics"
	"cposim/gateway/ocpipush"
	"cposim/gateway/random"
	"cposim/gateway/trace"
	"cposim/handler/api"
	ocpihandler "cposim/handler/ocpi"
	"cposim/handler/web"
	"cposim/mockemsp"
	"cposim/ocpi"
	cdrrepo "cposim/repository/cdr"
	chargerrepo "cposim/repository/charger"
	commandrepo "cposim/repository/command"
	sessionrepo "cposim/repository/session"
	siterepo "cposim/repository/site"
)

const (
	// All durations below are simulated time unless they say otherwise.
	advertisedCommandTimeout = 90 * time.Second
	commandLatency           = 2 * time.Second
	sessionUpdateInterval    = 30 * time.Second
	startTimeout             = 60 * time.Second

	currency    = "USD"
	countryCode = "US"
	partyID     = "SIM"
	timeZone    = "America/Los_Angeles"

	defaultMaxPowerKW  = 50
	defaultPricePerKWH = 0.45
	realTimeSpeed      = 1

	// Limits that keep a public, unauthenticated, in-memory instance bounded.
	maxChargers          = 50
	maxCompletedSessions = 500
	maxFinishedCommands  = 500
	maxSites             = 20
	maxSpeed             = 600

	pushQueueSize = 1024
	traceCapacity = 500
)

// A new charger is about as reliable as a real one unless it says otherwise.
var defaultBehaviors = []entity.BehaviorSpec{{Kind: behavior.KindRealisticReliability}}

var defaultVehicle = entity.Vehicle{
	BatteryCapacityKWH: 60,
	MaxPowerKW:         150,
	StateOfCharge:      0.2,
}

var mockDriverToken = ocpi.Token{
	ContractID:  "US-EMS-C0001",
	CountryCode: "US",
	PartyID:     "EMS",
	Type:        "APP_USER",
	UID:         "DRIVER-1",
}

// world is one complete, self-contained simulation: its own clock, state, OCPI adapter and
// (optionally) mock eMSP. Nothing in it is shared with another world except what it is handed
// here, so resetting the simulator is building a new world and letting go of the old one, and
// per-tester sandboxes would be a map of these.
type world struct {
	chargerController charger.Controller
	commandController command.Controller
	handler           http.Handler
	mockEMSP          mockemsp.Mock
	pushGateway       ocpipush.Gateway
	worldID           string
}

func newWorld(
	config Config,
	metricsGateway metrics.Gateway,
	out io.Writer,
	randomGateway random.Gateway,
	wallNow clock.NowFunc,
	worldID string,
) (*world, error) {
	initialSpeed := config.InitialSpeed
	if initialSpeed == 0 {
		initialSpeed = realTimeSpeed
	}

	clockGateway, err := clock.NewScaledGateway(maxSpeed, initialSpeed, wallNow)
	if err != nil {
		return nil, fmt.Errorf("clock.NewScaledGateway: %w", err)
	}

	chargerRepository := chargerrepo.NewInMemoryRepository()
	identifierGateway := identifier.NewSequentialGateway()
	mapper := ocpi.Mapper{
		CountryCode: countryCode,
		Currency:    currency,
		PartyID:     partyID,
		TimeZone:    timeZone,
	}
	siteRepository := siterepo.NewInMemoryRepository()
	traceGateway := trace.NewInMemoryGateway(traceCapacity)

	pushGateway, err := ocpipush.NewGateway(
		chargerRepository,
		ocpipush.Config{
			EMSPBaseURL: config.EMSPBaseURL,
			Mapper:      mapper,
			QueueSize:   pushQueueSize,
		},
		metricsGateway,
		out,
		ocpipush.NewHTTPSender(clockGateway, traceGateway),
		siteRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("ocpipush.NewGateway: %w", err)
	}

	sessionController, err := session.NewController(
		cdrrepo.NewInMemoryRepository(),
		clockGateway,
		session.Config{
			Currency:              currency,
			MaxCompletedSessions:  maxCompletedSessions,
			SessionUpdateInterval: sessionUpdateInterval,
		},
		pushGateway,
		identifierGateway,
		sessionrepo.NewInMemoryRepository(),
	)
	if err != nil {
		return nil, fmt.Errorf("session.NewController: %w", err)
	}

	chargerController, err := charger.NewController(
		chargerRepository,
		clockGateway,
		charger.Config{
			DefaultBehaviors:   defaultBehaviors,
			DefaultMaxPowerKW:  defaultMaxPowerKW,
			DefaultPricePerKWH: defaultPricePerKWH,
			DefaultVehicle:     defaultVehicle,
			MaxChargers:        maxChargers,
			MaxSites:           maxSites,
		},
		pushGateway,
		identifierGateway,
		randomGateway,
		sessionController,
		siteRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("charger.NewController: %w", err)
	}

	commandController, err := command.NewController(
		chargerController,
		clockGateway,
		commandrepo.NewInMemoryRepository(),
		command.Config{
			CommandLatency:      commandLatency,
			MaxFinishedCommands: maxFinishedCommands,
			StartTimeout:        startTimeout,
		},
		pushGateway,
		identifierGateway,
		metricsGateway,
		randomGateway,
	)
	if err != nil {
		return nil, fmt.Errorf("command.NewController: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(api.BasePath+"/", api.NewHandler(
		chargerController,
		clockGateway,
		commandController,
		api.Config{WorldID: worldID},
		metricsGateway,
		sessionController,
		traceGateway,
	))
	mux.Handle(ocpihandler.BasePath+"/", ocpihandler.NewHandler(
		chargerController,
		clockGateway,
		commandController,
		ocpihandler.Config{
			CommandTimeout: advertisedCommandTimeout,
			Mapper:         mapper,
		},
		sessionController,
		traceGateway,
	))

	webHandler, err := web.NewHandler()
	if err != nil {
		return nil, fmt.Errorf("web.NewHandler: %w", err)
	}
	mux.Handle("/", webHandler)

	// The mock eMSP is a guest in this process: it is handed URLs, not controllers.
	var mockEMSP mockemsp.Mock
	if config.MockEMSPSelfBaseURL != "" {
		mockEMSP = mockemsp.NewMock(
			mockemsp.Config{
				DriverToken: mockDriverToken,
				SelfBaseURL: config.MockEMSPSelfBaseURL,
			},
			mockemsp.NewHTTPCPOClient(config.MockEMSPSelfBaseURL),
		)
		mux.Handle(mockemsp.BasePath+"/", mockEMSP)
	}

	return &world{
		chargerController: chargerController,
		commandController: commandController,
		handler:           mux,
		mockEMSP:          mockEMSP,
		pushGateway:       pushGateway,
		worldID:           worldID,
	}, nil
}

// tick resolves commands before advancing chargers, so a session started in this tick is
// metered from its own start rather than from the previous tick.
func (w *world) tick() error {
	if err := w.commandController.Tick(); err != nil {
		return fmt.Errorf("commandController.Tick: %w", err)
	}

	if err := w.chargerController.Tick(); err != nil {
		return fmt.Errorf("chargerController.Tick: %w", err)
	}

	return nil
}

// connectMockEMSP has the mock eMSP pull the CPO's locations, as a real eMSP does when it first
// connects. It does nothing when the mock is not mounted.
func (w *world) connectMockEMSP() error {
	if w.mockEMSP == nil {
		return nil
	}

	if err := w.mockEMSP.SyncLocations(); err != nil {
		return fmt.Errorf("mockEMSP.SyncLocations: %w", err)
	}

	return nil
}
