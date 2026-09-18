// Package app is the dependency-injection root: the only place that constructs concrete
// implementations and holds the tunable constants.
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
	"cposim/gateway/ocpipush"
	"cposim/gateway/random"
	"cposim/gateway/scheduler"
	"cposim/gateway/trace"
	"cposim/handler/api"
	ocpihandler "cposim/handler/ocpi"
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
	initialSpeed       = 1

	pushQueueSize     = 1024
	simulationJobID   = "simulation"
	tickFrequencyWall = 250 * time.Millisecond
	traceCapacity     = 500
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

type Config struct {
	// Where OCPI pushes are delivered: the base URL of an eMSP's receiver endpoints.
	EMSPBaseURL string
	// When set, the bundled mock eMSP is mounted in this process. It is this server's own base
	// URL: the mock reaches the CPO through it, and gives it to the CPO as its callback address.
	MockEMSPSelfBaseURL string
	Out                 io.Writer
}

type Simulator interface {
	ConnectMockEMSP() error
	Handler() http.Handler
	Seed() error
	Start() error
	Stop() error
}

type simulator struct {
	chargerController charger.Controller
	commandController command.Controller
	handler           http.Handler
	mockEMSP          mockemsp.Mock
	pushGateway       ocpipush.Gateway
	schedulerGateway  scheduler.Gateway
}

func NewSimulator(config Config) (Simulator, error) {
	return NewSimulatorWithTicker(
		config,
		scheduler.NewRealTickerFunc(tickFrequencyWall),
		random.NewMathGateway(),
		time.Now,
	)
}

func NewSimulatorWithTicker(
	config Config,
	newTicker scheduler.NewTickerFunc,
	randomGateway random.Gateway,
	wallNow clock.NowFunc,
) (Simulator, error) {
	clockGateway, err := clock.NewScaledGateway(initialSpeed, wallNow)
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
		config.Out,
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
			CommandLatency: commandLatency,
			StartTimeout:   startTimeout,
		},
		pushGateway,
		identifierGateway,
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

	return &simulator{
		chargerController: chargerController,
		commandController: commandController,
		handler:           mux,
		mockEMSP:          mockEMSP,
		pushGateway:       pushGateway,
		schedulerGateway:  scheduler.NewTickerGateway(newTicker, config.Out),
	}, nil
}

// ConnectMockEMSP has the mock eMSP pull the CPO's locations, as a real eMSP does when it first
// connects. It does nothing when the mock is not mounted.
func (s *simulator) ConnectMockEMSP() error {
	if s.mockEMSP == nil {
		return nil
	}

	if err := s.mockEMSP.SyncLocations(); err != nil {
		return fmt.Errorf("mockEMSP.SyncLocations: %w", err)
	}

	return nil
}

func (s *simulator) Handler() http.Handler {
	return s.handler
}

func (s *simulator) Start() error {
	if err := s.schedulerGateway.StartScheduledJob(simulationJobID, s.tick); err != nil {
		return fmt.Errorf("schedulerGateway.StartScheduledJob: %w", err)
	}

	return nil
}

// tick resolves commands before advancing chargers, so a session started in this tick is
// metered from its own start rather than from the previous tick.
func (s *simulator) tick() error {
	if err := s.commandController.Tick(); err != nil {
		return fmt.Errorf("commandController.Tick: %w", err)
	}

	if err := s.chargerController.Tick(); err != nil {
		return fmt.Errorf("chargerController.Tick: %w", err)
	}

	return nil
}

func (s *simulator) Stop() error {
	if err := s.schedulerGateway.StopScheduledJob(simulationJobID); err != nil {
		return fmt.Errorf("schedulerGateway.StopScheduledJob: %w", err)
	}

	s.pushGateway.Stop()

	return nil
}
