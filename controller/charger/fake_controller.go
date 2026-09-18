package charger

import (
	"sync"

	"cposim/entity"
)

type PhysicalActionCall struct {
	Action    string
	ChargerID string
}

// FakeController records every charger mutation other than start/stop/unlock as a
// PhysicalActionCall named after the method, and answers them all with GetChargerResult.
type FakeController struct {
	GetChargerCalledWith      []string
	GetChargerErr             error
	GetChargerResult          entity.Charger
	GetSiteErr                error
	GetSiteResult             entity.Site
	ListChargersErr           error
	ListChargersResult        []entity.Charger
	ListSitesErr              error
	ListSitesResult           []entity.Site
	PhysicalActionCalls       []PhysicalActionCall
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
	return c.recordPhysicalAction("AddCharger", input.ChargerID)
}

func (c *FakeController) recordPhysicalAction(action string, chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.PhysicalActionCalls = append(c.PhysicalActionCalls, PhysicalActionCall{
		Action:    action,
		ChargerID: chargerID,
	})

	return c.GetChargerResult, c.PhysicalActionErr
}

func (c *FakeController) AddSite(site entity.Site) (entity.Site, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return site, c.PhysicalActionErr
}

func (c *FakeController) ClearFault(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction("ClearFault", chargerID)
}

func (c *FakeController) GetCharger(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetChargerCalledWith = append(c.GetChargerCalledWith, chargerID)

	return c.GetChargerResult, c.GetChargerErr
}

func (c *FakeController) GetSite(siteID string) (entity.Site, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.GetSiteResult, c.GetSiteErr
}

func (c *FakeController) InjectFault(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction("InjectFault", chargerID)
}

func (c *FakeController) ListChargers() ([]entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ListChargersResult, c.ListChargersErr
}

func (c *FakeController) ListSites() ([]entity.Site, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ListSitesResult, c.ListSitesErr
}

func (c *FakeController) PlugIn(chargerID string, vehicle entity.Vehicle) (entity.Charger, error) {
	return c.recordPhysicalAction("PlugIn", chargerID)
}

func (c *FakeController) PressStopButton(chargerID string) (entity.Charger, error) {
	return c.recordPhysicalAction("PressStopButton", chargerID)
}

func (c *FakeController) RemoveCharger(chargerID string) error {
	_, err := c.recordPhysicalAction("RemoveCharger", chargerID)

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
	return c.recordPhysicalAction("Unplug", chargerID)
}

func (c *FakeController) UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error) {
	return c.recordPhysicalAction("UpdateBehaviors", chargerID)
}
