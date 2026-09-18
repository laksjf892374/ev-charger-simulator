package app_test

import (
	"net/http"
	"testing"
	"time"

	"cposim/app/mockemsp"
	"cposim/gateway/random"
	"cposim/internal/assert"
)

// Scenarios are whole user stories, driven only through the public HTTP APIs of a running
// simulator with the mock eMSP mounted: what a person at a charger, a driver with a phone, and an
// operator would actually do, and what each of them should see.

func TestOperatorMistakeScenarios(t *testing.T) {
	t.Run("refuses what the physical world would refuse, and says why", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)

		// When / Then
		assert.Equal(t, f.atCharger(t, validFastChargerID, "unplug", ""), http.StatusConflict)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "plug-in", ""), http.StatusConflict)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "clear-fault", ""), http.StatusConflict)
		assert.Equal(t, f.atCharger(t, validHubChargerID, "press-stop", ""), http.StatusConflict)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "kick", ""), http.StatusNotFound)
		assert.Equal(t, f.atCharger(t, "NOPE", "plug-in", ""), http.StatusNotFound)
	})

	t.Run("refuses configuration that makes no sense, and leaves the world unchanged", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		before := f.world(t)

		// When / Then
		for _, attempt := range []struct{ method, path, body string }{
			{http.MethodPost, "/api/chargers", `{"site_id": "SITE-MISSING"}`},
			{http.MethodPost, "/api/chargers", `{"site_id": "` + validHubSiteID + `", "charger_id": "../../etc"}`},
			{http.MethodPost, "/api/chargers", `{"site_id": "` + validHubSiteID + `", "max_power_kw": 1e9}`},
			{http.MethodPost, "/api/chargers", `{"site_id": "` + validHubSiteID + `", "behaviors": [{"kind": "start_fails", "params": {"delay_s": -5}}]}`},
			{http.MethodPut, "/api/chargers/" + validFastChargerID + "/behaviors", `[{"kind": "made_up"}]`},
			{http.MethodPut, "/api/clock", `{"speed": 100000}`},
			{http.MethodPut, "/api/clock", `{"speed": 0}`},
			{http.MethodPost, "/api/sites", `{"name": "Nowhere", "latitude": 123}`},
		} {
			statusCode, body := f.request(t, attempt.method, attempt.path, attempt.body)
			if statusCode != http.StatusUnprocessableEntity {
				t.Fatalf("%s %s %s answered %d, want 422: %s", attempt.method, attempt.path, attempt.body, statusCode, body)
			}
		}

		assert.Equal(t, f.world(t).Chargers, before.Chargers)
	})

	t.Run("stops accepting chargers at the limit instead of growing without bound", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		lastStatusCode, lastBody, added := 0, "", 0
		for added = 0; added < 100; added++ {
			lastStatusCode, lastBody = f.request(t, http.MethodPost, "/api/chargers", `{"site_id": "`+validHubSiteID+`"}`)
			if lastStatusCode != http.StatusCreated {
				break
			}
		}

		// Then
		assert.Equal(t, lastStatusCode, http.StatusUnprocessableEntity)
		assert.Contains(t, lastBody, "charger limit reached: limit 50")
		assert.Equal(t, added, 47)
	})
}

func TestFirstVisitScenarios(t *testing.T) {
	t.Run("the demo world has a charger that never fails, an everyday one, and one set up to fail", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		_, body := f.request(t, http.MethodGet, "/api/chargers", "")

		// Then
		assert.Contains(t, body, `"behaviors":[],"charger_id":"EVSE-000001"`)
		assert.Contains(t, body, `"behaviors":[{"kind":"realistic_reliability"}],"charger_id":"EVSE-000002"`)
		assert.Contains(t, body, `"behaviors":[{"kind":"start_fails","params":{"delay_s":8}}],"charger_id":"EVSE-000003"`)
	})

	t.Run("a first try on Charger 1 works even when every roll of the dice is unlucky", func(t *testing.T) {
		// Given
		unluckyRandomGateway := random.NewFakeGateway()
		f := newMockEMSPFixtureWithRandom(t, unluckyRandomGateway)

		// When
		sessionID := f.startCharging(t, validHubSiteID, validHubChargerID, defaultVehicle)
		f.advance(t, 10*time.Minute)
		f.phoneStop(t, sessionID)
		f.advance(t, 2*time.Second)

		// Then
		believed := f.phoneSees(t, "the first charge was billed", func(believed mockemsp.State) bool { return len(believed.CDRs) == 1 })
		assert.Equal(t, believed.Sessions[0].Status, "COMPLETED")
	})
}
