package charger

import (
	"sync"

	"cposim/entity"
)

// FakeController covers what collaborating controllers call. The physical actions only record
// their charger ID; extend it when a test needs more.
type FakeController struct {
	GetChargerCalledWith      []string
	GetChargerErr             error
	GetChargerResult          entity.Charger
	PhysicalActionCalledWith  []string
	PhysicalActionErr         error
	StartChargingCalledWith   []StartChargingInput
	StartChargingErr          error
	StartChargingResult       entity.Session
	StopChargingCalledWith    []string
	StopChargingErr           error
	StopChargingResult        entity.Session
	TickCallCount             int
	TickErr                   error
	UnlockConnectorCalledWith []string
	UnlockConnectorErr        error
	mu                        sync.Mutex
}

func NewFakeController() *FakeController {
	return &FakeController{}
}

func (c *FakeController) AddCharger(input AddChargerInput) (entity.Charger, error) {
	return c.recordPhysicalAction(input.ChargerID)
}

func (c *FakeController) recordPhysicalAction(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.PhysicalActionCalledWith = append(c.PhysicalActionCalledWith, chargerID)

	return c.GetChargerResult, c.PhysicalActionErr
}

func (c *FakeController) AddSite(site entity.Site) (entity.Site, error) {
	return site, nil
}

func (c *FakeController) ClearFault(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}

func (c *FakeController) GetCharger(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetChargerCalledWith = append(c.GetChargerCalledWith, chargerID)

	return c.GetChargerResult, c.GetChargerErr
}

func (c *FakeController) GetSite(siteID string) (entity.Site, error) {
	return entity.Site{SiteID: siteID}, nil
}

func (c *FakeController) InjectFault(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}

func (c *FakeController) ListChargers() ([]entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return []entity.Charger{c.GetChargerResult}, c.GetChargerErr
}

func (c *FakeController) ListSites() ([]entity.Site, error) {
	return nil, nil
}

func (c *FakeController) PlugIn(chargerID string, vehicle entity.Vehicle) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}

func (c *FakeController) PressStopButton(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}

func (c *FakeController) RemoveCharger(chargerID string) error {
	_, err := c.recordPhysicalAction(chargerID)

	return err
}

func (c *FakeController) StartCharging(input StartChargingInput) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StartChargingCalledWith = append(c.StartChargingCalledWith, input)

	return c.StartChargingResult, c.StartChargingErr
}

func (c *FakeController) StopCharging(sessionID string) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StopChargingCalledWith = append(c.StopChargingCalledWith, sessionID)

	return c.StopChargingResult, c.StopChargingErr
}

func (c *FakeController) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.TickCallCount++

	return c.TickErr
}

func (c *FakeController) UnlockConnector(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.UnlockConnectorCalledWith = append(c.UnlockConnectorCalledWith, chargerID)

	return c.GetChargerResult, c.UnlockConnectorErr
}

func (c *FakeController) Unplug(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}

func (c *FakeController) UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error) {
	return c.recordPhysicalAction(chargerID)
}
