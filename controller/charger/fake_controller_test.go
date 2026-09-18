package charger_test

import (
	"errors"
	"testing"

	"cposim/assert"
	"cposim/controller/charger"
	"cposim/entity"
)

func TestFakeController(t *testing.T) {
	t.Run("returns the configured errors", func(t *testing.T) {
		// Given
		fakeController := charger.NewFakeController()
		fakeController.GetChargerErr = errors.New("boom")
		fakeController.GetSiteErr = errors.New("boom")
		fakeController.ListChargersErr = errors.New("boom")
		fakeController.ListSitesErr = errors.New("boom")
		fakeController.PhysicalActionErr = errors.New("boom")
		fakeController.StartChargingErr = errors.New("boom")
		fakeController.StopChargingErr = errors.New("boom")
		fakeController.TickErr = errors.New("boom")
		fakeController.UnlockConnectorErr = errors.New("boom")

		// When
		_, getChargerErr := fakeController.GetCharger("CH-1")
		_, getSiteErr := fakeController.GetSite("SITE-1")
		_, listChargersErr := fakeController.ListChargers()
		_, listSitesErr := fakeController.ListSites()
		_, plugInErr := fakeController.PlugIn("CH-1", entity.Vehicle{})
		_, startChargingErr := fakeController.StartCharging(charger.StartChargingInput{})
		_, stopChargingErr := fakeController.StopCharging("SES-1")
		tickErr := fakeController.Tick()
		_, unlockConnectorErr := fakeController.UnlockConnector("CH-1")

		// Then
		assert.Error(t, getChargerErr)
		assert.Error(t, getSiteErr)
		assert.Error(t, listChargersErr)
		assert.Error(t, listSitesErr)
		assert.Error(t, plugInErr)
		assert.Error(t, startChargingErr)
		assert.Error(t, stopChargingErr)
		assert.Error(t, tickErr)
		assert.Error(t, unlockConnectorErr)
	})

	t.Run("records calls and returns the configured results", func(t *testing.T) {
		// Given
		fakeController := charger.NewFakeController()
		fakeController.GetChargerResult = entity.Charger{ChargerID: "CH-1"}
		fakeController.GetSiteResult = entity.Site{SiteID: "SITE-1"}
		fakeController.ListChargersResult = []entity.Charger{{ChargerID: "CH-9"}}
		fakeController.ListSitesResult = []entity.Site{{SiteID: "SITE-9"}}
		fakeController.StartChargingResult = entity.Session{SessionID: "SES-1"}
		fakeController.StopChargingResult = entity.Session{SessionID: "SES-2"}

		// When
		gotCharger, err := fakeController.GetCharger("CH-1")
		assert.NoError(t, err)
		_, err = fakeController.PlugIn("CH-2", entity.Vehicle{})
		assert.NoError(t, err)
		startedSession, err := fakeController.StartCharging(charger.StartChargingInput{ChargerID: "CH-1"})
		assert.NoError(t, err)
		stoppedSession, err := fakeController.StopCharging("SES-2")
		assert.NoError(t, err)
		assert.NoError(t, fakeController.Tick())
		_, err = fakeController.UnlockConnector("CH-3")
		assert.NoError(t, err)

		gotSite, err := fakeController.GetSite("SITE-1")
		assert.NoError(t, err)
		chargers, err := fakeController.ListChargers()
		assert.NoError(t, err)
		sites, err := fakeController.ListSites()
		assert.NoError(t, err)

		// Then
		assert.Equal(t, gotSite, entity.Site{SiteID: "SITE-1"})
		assert.Equal(t, chargers, []entity.Charger{{ChargerID: "CH-9"}})
		assert.Equal(t, sites, []entity.Site{{SiteID: "SITE-9"}})
		assert.Equal(t, gotCharger, entity.Charger{ChargerID: "CH-1"})
		assert.Equal(t, fakeController.GetChargerCalledWith, []string{"CH-1"})
		assert.Equal(t, fakeController.PhysicalActionCalledWith, []string{"CH-2"})
		assert.Equal(t, startedSession, entity.Session{SessionID: "SES-1"})
		assert.Equal(t, fakeController.StartChargingCalledWith, []charger.StartChargingInput{{ChargerID: "CH-1"}})
		assert.Equal(t, stoppedSession, entity.Session{SessionID: "SES-2"})
		assert.Equal(t, fakeController.StopChargingCalledWith, []string{"SES-2"})
		assert.Equal(t, fakeController.TickCallCount, 1)
		assert.Equal(t, fakeController.UnlockConnectorCalledWith, []string{"CH-3"})
	})
}
