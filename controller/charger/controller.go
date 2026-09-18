package charger

import (
	"fmt"
	"math"
	"sync"
	"time"

	"cposim/behavior"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	chargerrepo "cposim/repository/charger"
	siterepo "cposim/repository/site"
)

const (
	chargerIDPrefix = "EVSE"
	siteIDPrefix    = "SITE"

	// Above this state of charge a vehicle tapers linearly from full power down to
	// taperFloorPowerFactor at 100%.
	taperFloorPowerFactor   = 0.1
	taperStartStateOfCharge = 0.8
)

type Config struct {
	DefaultMaxPowerKW  float64
	DefaultPricePerKWH float64
	DefaultVehicle     entity.Vehicle
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
	sessionController session.Controller,
	siteRepository siterepo.Repository,
) (Controller, error) {
	if config.DefaultMaxPowerKW <= 0 {
		return nil, fmt.Errorf("default max power must be positive: power %v kW", config.DefaultMaxPowerKW)
	}

	if config.DefaultPricePerKWH < 0 {
		return nil, fmt.Errorf("default price must not be negative: price %v", config.DefaultPricePerKWH)
	}

	if err := validateVehicle(config.DefaultVehicle); err != nil {
		return nil, fmt.Errorf("validateVehicle: %w", err)
	}

	return &controller{
		chargerRepository: chargerRepository,
		clockGateway:      clockGateway,
		config:            config,
		eventsGateway:     eventsGateway,
		identifierGateway: identifierGateway,
		lastTickAt:        clockGateway.Now(),
		sessionController: sessionController,
		siteRepository:    siteRepository,
	}, nil
}

func validateVehicle(vehicle entity.Vehicle) error {
	if vehicle.BatteryCapacityKWH <= 0 {
		return fmt.Errorf("vehicle battery capacity must be positive: capacity %v kWh", vehicle.BatteryCapacityKWH)
	}

	if vehicle.MaxPowerKW <= 0 {
		return fmt.Errorf("vehicle max power must be positive: power %v kW", vehicle.MaxPowerKW)
	}

	if vehicle.StateOfCharge < 0 || vehicle.StateOfCharge >= 1 {
		return fmt.Errorf("vehicle state of charge must be in [0, 1): state of charge %v", vehicle.StateOfCharge)
	}

	return nil
}

func (c *controller) AddSite(site entity.Site) (entity.Site, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if site.Name == "" {
		return entity.Site{}, fmt.Errorf("site name must not be empty")
	}

	if site.SiteID == "" {
		site.SiteID = c.identifierGateway.NewID(siteIDPrefix)
	}

	site.UpdatedAt = c.clockGateway.Now()

	if err := c.siteRepository.Upsert(site); err != nil {
		return entity.Site{}, fmt.Errorf("siteRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishSiteEvent(site); err != nil {
		return entity.Site{}, fmt.Errorf("eventsGateway.PublishSiteEvent: %w", err)
	}

	return site, nil
}

func (c *controller) GetSite(siteID string) (entity.Site, error) {
	site, err := c.siteRepository.Get(siteID)
	if err != nil {
		return entity.Site{}, fmt.Errorf("siteRepository.Get: %w", err)
	}

	return site, nil
}

func (c *controller) ListSites() ([]entity.Site, error) {
	sites, err := c.siteRepository.List()
	if err != nil {
		return nil, fmt.Errorf("siteRepository.List: %w", err)
	}

	return sites, nil
}

func (c *controller) AddCharger(input AddChargerInput) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.siteRepository.Get(input.SiteID); err != nil {
		return entity.Charger{}, fmt.Errorf("siteRepository.Get: %w", err)
	}

	if _, err := behavior.Build(input.Behaviors); err != nil {
		return entity.Charger{}, fmt.Errorf("behavior.Build: %w", err)
	}

	if input.MaxPowerKW < 0 || input.PricePerKWH < 0 {
		return entity.Charger{}, fmt.Errorf("max power and price must not be negative")
	}

	charger := entity.Charger{
		Behaviors:   input.Behaviors,
		ChargerID:   input.ChargerID,
		MaxPowerKW:  input.MaxPowerKW,
		PricePerKWH: input.PricePerKWH,
		SiteID:      input.SiteID,
		State:       entity.ChargerStateAvailable,
	}

	if charger.ChargerID == "" {
		charger.ChargerID = c.identifierGateway.NewID(chargerIDPrefix)
	} else if _, err := c.chargerRepository.Get(charger.ChargerID); err == nil {
		return entity.Charger{}, fmt.Errorf("charger %q already exists", charger.ChargerID)
	}

	if charger.MaxPowerKW == 0 {
		charger.MaxPowerKW = c.config.DefaultMaxPowerKW
	}

	if charger.PricePerKWH == 0 {
		charger.PricePerKWH = c.config.DefaultPricePerKWH
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) updateCharger(charger *entity.Charger) error {
	charger.UpdatedAt = c.clockGateway.Now()

	if err := c.chargerRepository.Upsert(*charger); err != nil {
		return fmt.Errorf("chargerRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishChargerEvent(*charger); err != nil {
		return fmt.Errorf("eventsGateway.PublishChargerEvent: %w", err)
	}

	return nil
}

func (c *controller) RemoveCharger(chargerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != "" {
		return fmt.Errorf("charger %q has an active session: session %q", chargerID, charger.SessionID)
	}

	if err := c.chargerRepository.Delete(chargerID); err != nil {
		return fmt.Errorf("chargerRepository.Delete: %w", err)
	}

	charger.UpdatedAt = c.clockGateway.Now()

	if err := c.eventsGateway.PublishChargerRemovedEvent(charger); err != nil {
		return fmt.Errorf("eventsGateway.PublishChargerRemovedEvent: %w", err)
	}

	return nil
}

func (c *controller) UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if _, err := behavior.Build(behaviors); err != nil {
		return entity.Charger{}, fmt.Errorf("behavior.Build: %w", err)
	}

	charger.Behaviors = behaviors

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) GetCharger(chargerID string) (entity.Charger, error) {
	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	return charger, nil
}

func (c *controller) ListChargers() ([]entity.Charger, error) {
	chargers, err := c.chargerRepository.List()
	if err != nil {
		return nil, fmt.Errorf("chargerRepository.List: %w", err)
	}

	return chargers, nil
}

func (c *controller) PlugIn(chargerID string, vehicle entity.Vehicle) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle != nil {
		return entity.Charger{}, fmt.Errorf("charger %q already has a vehicle plugged in", chargerID)
	}

	if vehicle == (entity.Vehicle{}) {
		vehicle = c.config.DefaultVehicle
	}

	if err := validateVehicle(vehicle); err != nil {
		return entity.Charger{}, fmt.Errorf("validateVehicle: %w", err)
	}

	charger.Vehicle = &vehicle
	if charger.State == entity.ChargerStateAvailable {
		charger.State = entity.ChargerStatePreparing
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) Unplug(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle == nil {
		return entity.Charger{}, fmt.Errorf("charger %q has no vehicle plugged in", chargerID)
	}

	if charger.ConnectorLocked {
		return entity.Charger{}, fmt.Errorf("charger %q has its connector locked: charger state %q", chargerID, charger.State)
	}

	charger.Vehicle = nil
	if charger.State != entity.ChargerStateFaulted {
		charger.State = entity.ChargerStateAvailable
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) StartCharging(input StartChargingInput) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(input.ChargerID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle == nil {
		return entity.Session{}, fmt.Errorf("charger %q has no vehicle plugged in", input.ChargerID)
	}

	if charger.State != entity.ChargerStatePreparing && charger.State != entity.ChargerStateFinishing {
		return entity.Session{}, fmt.Errorf("charger %q is not ready to charge: charger state %q", input.ChargerID, charger.State)
	}

	startedSession, err := c.sessionController.StartSession(session.StartSessionInput{
		AuthorizationReference: input.AuthorizationReference,
		ChargerID:              charger.ChargerID,
		PricePerKWH:            charger.PricePerKWH,
		SiteID:                 charger.SiteID,
		Token:                  input.Token,
	})
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.StartSession: %w", err)
	}

	charger.ConnectorLocked = true
	charger.SessionID = startedSession.SessionID
	charger.State = entity.ChargerStateCharging

	if err := c.updateCharger(&charger); err != nil {
		return entity.Session{}, fmt.Errorf("updateCharger: %w", err)
	}

	return startedSession, nil
}

func (c *controller) StopCharging(sessionID string) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	activeSession, err := c.sessionController.GetSession(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.GetSession: %w", err)
	}

	charger, err := c.chargerRepository.Get(activeSession.ChargerID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != sessionID {
		return entity.Session{}, fmt.Errorf("session %q is not active on charger %q", sessionID, charger.ChargerID)
	}

	stoppedSession, err := c.endSession(&charger, entity.StopReasonRemote, entity.ChargerStateFinishing)
	if err != nil {
		return entity.Session{}, fmt.Errorf("endSession: %w", err)
	}

	return stoppedSession, nil
}

// endSession stops the charger's active session and moves the charger to nextState. A faulted
// charger keeps its connector locked: the stuck cable is part of the scenario.
func (c *controller) endSession(
	charger *entity.Charger,
	stopReason entity.StopReason,
	nextState entity.ChargerState,
) (entity.Session, error) {
	stoppedSession, err := c.sessionController.StopSession(charger.SessionID, stopReason)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.StopSession: %w", err)
	}

	if charger.Vehicle != nil {
		vehicle := *charger.Vehicle
		vehicle.StateOfCharge = stateOfChargeAfter(vehicle, stoppedSession.EnergyDeliveredKWH)
		charger.Vehicle = &vehicle
	}

	charger.ConnectorLocked = nextState == entity.ChargerStateFaulted
	charger.SessionID = ""
	charger.State = nextState

	if err := c.updateCharger(charger); err != nil {
		return entity.Session{}, fmt.Errorf("updateCharger: %w", err)
	}

	return stoppedSession, nil
}

func stateOfChargeAfter(vehicle entity.Vehicle, energyDeliveredKWH float64) float64 {
	return math.Min(1, vehicle.StateOfCharge+energyDeliveredKWH/vehicle.BatteryCapacityKWH)
}

func (c *controller) PressStopButton(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID == "" {
		return entity.Charger{}, fmt.Errorf("charger %q has no active session: charger state %q", chargerID, charger.State)
	}

	if _, err := c.endSession(&charger, entity.StopReasonStopButton, entity.ChargerStateFinishing); err != nil {
		return entity.Charger{}, fmt.Errorf("endSession: %w", err)
	}

	return charger, nil
}

func (c *controller) InjectFault(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.State == entity.ChargerStateFaulted {
		return entity.Charger{}, fmt.Errorf("charger %q is already faulted", chargerID)
	}

	if err := c.fault(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("fault: %w", err)
	}

	return charger, nil
}

func (c *controller) fault(charger *entity.Charger) error {
	if charger.SessionID != "" {
		if _, err := c.endSession(charger, entity.StopReasonFault, entity.ChargerStateFaulted); err != nil {
			return fmt.Errorf("endSession: %w", err)
		}

		return nil
	}

	charger.State = entity.ChargerStateFaulted

	if err := c.updateCharger(charger); err != nil {
		return fmt.Errorf("updateCharger: %w", err)
	}

	return nil
}

func (c *controller) ClearFault(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.State != entity.ChargerStateFaulted {
		return entity.Charger{}, fmt.Errorf("charger %q is not faulted: charger state %q", chargerID, charger.State)
	}

	charger.ConnectorLocked = false
	charger.State = entity.ChargerStateAvailable
	if charger.Vehicle != nil {
		charger.State = entity.ChargerStatePreparing
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) UnlockConnector(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != "" {
		return entity.Charger{}, fmt.Errorf("charger %q has an active session: session %q", chargerID, charger.SessionID)
	}

	charger.ConnectorLocked = false

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

// Tick advances every charging charger by the simulated time elapsed since the previous tick.
// One tick drives all chargers (rather than one scheduled job per session) so a session can end
// itself from inside the tick, and so locks are only ever taken charger → session.
func (c *controller) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clockGateway.Now()
	elapsed := now.Sub(c.lastTickAt)
	c.lastTickAt = now

	chargers, err := c.chargerRepository.List()
	if err != nil {
		return fmt.Errorf("chargerRepository.List: %w", err)
	}

	for _, charger := range chargers {
		if charger.State != entity.ChargerStateCharging {
			continue
		}

		if err := c.advanceCharging(charger, elapsed, now); err != nil {
			return fmt.Errorf("advanceCharging: %w", err)
		}
	}

	return nil
}

func (c *controller) advanceCharging(charger entity.Charger, elapsed time.Duration, now time.Time) error {
	activeSession, err := c.sessionController.GetSession(charger.SessionID)
	if err != nil {
		return fmt.Errorf("sessionController.GetSession: %w", err)
	}

	tick := behavior.Tick{
		Charger:          charger,
		ChargingDuration: now.Sub(activeSession.StartedAt),
		PowerFactor:      1,
		Session:          activeSession,
	}
	if err := behavior.ApplyTickInterceptors(charger.Behaviors, &tick); err != nil {
		return fmt.Errorf("behavior.ApplyTickInterceptors: %w", err)
	}

	if tick.Fault {
		if err := c.fault(&charger); err != nil {
			return fmt.Errorf("fault: %w", err)
		}

		return nil
	}

	vehicle := *charger.Vehicle
	stateOfCharge := stateOfChargeAfter(vehicle, activeSession.EnergyDeliveredKWH)
	powerKW := deliveredPowerKW(charger, vehicle, stateOfCharge) * tick.PowerFactor
	energyKWH := powerKW * elapsed.Hours()

	remainingKWH := (1 - stateOfCharge) * vehicle.BatteryCapacityKWH
	vehicleFull := energyKWH >= remainingKWH
	if vehicleFull {
		energyKWH = remainingKWH
	}

	if _, err := c.sessionController.RecordSessionProgress(charger.SessionID, energyKWH, powerKW); err != nil {
		return fmt.Errorf("sessionController.RecordSessionProgress: %w", err)
	}

	if !vehicleFull {
		return nil
	}

	if _, err := c.endSession(&charger, entity.StopReasonVehicleFull, entity.ChargerStateFinishing); err != nil {
		return fmt.Errorf("endSession: %w", err)
	}

	return nil
}

func deliveredPowerKW(charger entity.Charger, vehicle entity.Vehicle, stateOfCharge float64) float64 {
	powerKW := math.Min(charger.MaxPowerKW, vehicle.MaxPowerKW)
	if stateOfCharge <= taperStartStateOfCharge {
		return powerKW
	}

	taperProgress := (stateOfCharge - taperStartStateOfCharge) / (1 - taperStartStateOfCharge)

	return powerKW * (1 - taperProgress*(1-taperFloorPowerFactor))
}
