package charger

import (
	"sync"

	"cposim/entity"
)

type MutationCall struct {
	ID     string
	Method string
}

// FakeController records StartCharging, StopCharging and UnlockConnector per method, because
// collaborating controllers depend on their arguments and results. Every other mutation is
// recorded as a MutationCall (the method's name and the ID it was aimed at) and answered with
// GetChargerResult and MutationErr: enough for a handler test to assert the right method was
// reached. Give a method its own fields when a test needs its arguments.
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
	MutationCalls             []MutationCall
	MutationErr               error
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
	return c.recordMutation("AddCharger", input.ChargerID)
}

func (c *FakeController) recordMutation(method string, id string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.MutationCalls = append(c.MutationCalls, MutationCall{
		ID:     id,
		Method: method,
	})

	return c.GetChargerResult, c.MutationErr
}

func (c *FakeController) AddSite(site entity.Site) (entity.Site, error) {
	_, err := c.recordMutation("AddSite", site.SiteID)

	return site, err
}

func (c *FakeController) ClearFault(chargerID string) (entity.Charger, error) {
	return c.recordMutation("ClearFault", chargerID)
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
	return c.recordMutation("InjectFault", chargerID)
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
	return c.recordMutation("PlugIn", chargerID)
}

func (c *FakeController) PressStopButton(chargerID string) (entity.Charger, error) {
	return c.recordMutation("PressStopButton", chargerID)
}

func (c *FakeController) RemoveCharger(chargerID string) error {
	_, err := c.recordMutation("RemoveCharger", chargerID)

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
	return c.recordMutation("Unplug", chargerID)
}

func (c *FakeController) UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error) {
	return c.recordMutation("UpdateBehaviors", chargerID)
}
