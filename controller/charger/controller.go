package charger

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"cposim/controller/behavior"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/random"
	chargerrepo "cposim/repository/charger"
	siterepo "cposim/repository/site"
)

const (
	chargerIDPrefix = "EVSE"
	siteIDPrefix    = "SITE"

	// Bounds on what a caller may configure. They keep the simulation physically plausible and a
	// public instance from being filled with junk.
	maxBatteryCapacityKWH = 1000
	maxPowerKW            = 1000
	maxPricePerKWH        = 100
	maxTextLength         = 200

	// Above this state of charge a vehicle tapers linearly from full power down to
	// taperFloorPowerFactor at 100%.
	taperFloorPowerFactor   = 0.1
	taperStartStateOfCharge = 0.8
)

// IDs end up in URL paths and in OCPI fields limited to 36 characters.
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,36}$`)

type Config struct {
	// Given to a new charger that does not say how it should behave. An explicitly empty list
	// still means "perfectly reliable".
	DefaultBehaviors   []entity.BehaviorSpec
	DefaultMaxPowerKW  float64
	DefaultPricePerKWH float64
	DefaultVehicle     entity.Vehicle
	MaxChargers        int
	MaxSites           int
}

type AddChargerInput struct {
	Behaviors   []entity.BehaviorSpec `json:"behaviors"`
	ChargerID   string                `json:"charger_id"`
	MaxPowerKW  float64               `json:"max_power_kw"`
	PricePerKWH float64               `json:"price_per_kwh"`
	SiteID      string                `json:"site_id"`
}

type StartChargingInput struct {
	AuthorizationReference string
	ChargerID              string
	Token                  entity.Token
}

type Controller interface {
	AddCharger(input AddChargerInput) (entity.Charger, error)
	AddSite(site entity.Site) (entity.Site, error)
	ClearFault(chargerID string) (entity.Charger, error)
	GetCharger(chargerID string) (entity.Charger, error)
	GetSite(siteID string) (entity.Site, error)
	InjectFault(chargerID string) (entity.Charger, error)
	ListChargers() ([]entity.Charger, error)
	ListSites() ([]entity.Site, error)
	PlugIn(chargerID string, vehicle entity.Vehicle) (entity.Charger, error)
	PressStopButton(chargerID string) (entity.Charger, error)
	RemoveCharger(chargerID string) error
	StartCharging(input StartChargingInput) (entity.Session, error)
	StopCharging(sessionID string) (entity.Session, error)
	Tick() error
	UnlockConnector(chargerID string) (entity.Charger, error)
	Unplug(chargerID string) (entity.Charger, error)
	UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error)
}

type controller struct {
	chargerRepository chargerrepo.Repository
	clockGateway      clock.Gateway
	config            Config
	eventsGateway     events.Gateway
	identifierGateway identifier.Gateway
	lastTickAt        time.Time
	randomGateway     random.Gateway
	sessionController session.Controller
	siteRepository    siterepo.Repository
	mu                sync.Mutex
}

func NewController(
	chargerRepository chargerrepo.Repository,
	clockGateway clock.Gateway,
	config Config,
	eventsGateway events.Gateway,
	identifierGateway identifier.Gateway,
	randomGateway random.Gateway,
	sessionController session.Controller,
	siteRepository siterepo.Repository,
) (Controller, error) {
	if _, err := behavior.Build(config.DefaultBehaviors); err != nil {
		return nil, fmt.Errorf("behavior.Build: %w", err)
	}

	if config.DefaultMaxPowerKW <= 0 {
		return nil, fmt.Errorf("default max power must be positive: power %v kW", config.DefaultMaxPowerKW)
	}

	if config.DefaultPricePerKWH < 0 {
		return nil, fmt.Errorf("default price must not be negative: price %v", config.DefaultPricePerKWH)
	}

	if err := validateVehicle(config.DefaultVehicle); err != nil {
		return nil, fmt.Errorf("validateVehicle: %w", err)
	}

	if config.MaxChargers <= 0 || config.MaxSites <= 0 {
		return nil, fmt.Errorf("charger and site limits must be positive: limits %d and %d", config.MaxChargers, config.MaxSites)
	}

	return &controller{
		chargerRepository: chargerRepository,
		clockGateway:      clockGateway,
		config:            config,
		eventsGateway:     eventsGateway,
		identifierGateway: identifierGateway,
		lastTickAt:        clockGateway.Now(),
		randomGateway:     randomGateway,
		sessionController: sessionController,
		siteRepository:    siteRepository,
	}, nil
}

func validateVehicle(vehicle entity.Vehicle) error {
	if vehicle.BatteryCapacityKWH <= 0 || vehicle.BatteryCapacityKWH > maxBatteryCapacityKWH {
		return fmt.Errorf("vehicle battery capacity must be above 0 and at most %d kWh: capacity %v kWh", maxBatteryCapacityKWH, vehicle.BatteryCapacityKWH)
	}

	if vehicle.MaxPowerKW <= 0 || vehicle.MaxPowerKW > maxPowerKW {
		return fmt.Errorf("vehicle max power must be above 0 and at most %d kW: power %v kW", maxPowerKW, vehicle.MaxPowerKW)
	}

	if vehicle.StateOfCharge < 0 || vehicle.StateOfCharge >= 1 {
		return fmt.Errorf("vehicle state of charge must be in [0, 1): state of charge %v", vehicle.StateOfCharge)
	}

	return nil
}
