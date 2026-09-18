package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
)

// Scenarios are whole user stories, driven only through the public HTTP APIs of a running
// simulator with the mock eMSP mounted: what a person at a charger, a driver with a phone, and an
// operator would actually do, and what each of them should see.

func TestOperationsScenarios(t *testing.T) {
	t.Run("reset throws the world away, for the CPO and for the phone, and starts again from the demo data", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)
		assert.Equal(t, f.atCharger(t, validHubChargerID, "inject-fault", ""), http.StatusOK)
		f.request(t, http.MethodPut, "/api/clock", `{"speed": 60}`)
		before := f.world(t)

		// When
		statusCode, body := f.request(t, http.MethodPost, "/api/reset", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		after := f.world(t)
		assert.NotEqual(t, after.WorldID, before.WorldID)
		assert.Contains(t, body, after.WorldID)
		chargerCount := len(after.Chargers)
		assert.Equal(t, chargerCount, 3)
		for _, resetCharger := range after.Chargers {
			assert.Equal(t, resetCharger.State, "AVAILABLE")
		}
		sessionCount := len(after.Sessions)
		assert.Equal(t, sessionCount, 0)
		_, clockBody := f.request(t, http.MethodGet, "/api/clock", "")
		assert.Contains(t, clockBody, `"speed":1`)

		believed := f.phoneSees(t, "the phone knows only the fresh world", func(believed mockemsp.State) bool {
			return evseStatus(believed, validFlakyChargerID) == "AVAILABLE"
		})
		phoneSessionCount := len(believed.Sessions)
		assert.Equal(t, phoneSessionCount, 0)
		phoneCommandCount := len(believed.Commands)
		assert.Equal(t, phoneCommandCount, 0)

		// When
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)

		// Then
		assert.Equal(t, f.world(t).Sessions[0].SessionID, "SES-000001")
	})

	t.Run("health turns red when the simulation clock stops, even though HTTP still answers", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.advance(t, time.Second)

		// When
		statusCode, body := f.request(t, http.MethodGet, "/healthz", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		assert.Contains(t, body, `"status":"ok"`)

		// When
		f.wallClock.Advance(6 * time.Second)
		statusCode, body = f.request(t, http.MethodGet, "/healthz", "")

		// Then
		assert.Equal(t, statusCode, http.StatusServiceUnavailable)
		assert.Contains(t, body, "simulation clock has stopped")

		// When
		f.advance(t, 0)
		statusCode, _ = f.request(t, http.MethodGet, "/healthz", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
	})

	t.Run("metrics account for what happened", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		sessionID := f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)
		f.phoneStop(t, sessionID)
		f.advance(t, 2*time.Second)
		f.phoneSees(t, "the session was billed", func(believed mockemsp.State) bool { return len(believed.CDRs) == 1 })

		// When
		_, body := f.request(t, http.MethodGet, "/api/metrics", "")

		// Then
		assert.Contains(t, body, `"commands_resolved_total_ACCEPTED":2`)
		assert.Contains(t, body, `"chargers":3`)
		assert.Contains(t, body, `"chargers_finishing":1`)
		assert.Contains(t, body, `"cdrs_kept":1`)
		assert.NotContains(t, body, `"pushes_failed_total"`)
		assert.NotContains(t, body, `"tick_errors_total"`)
		assert.Contains(t, body, `"pushes_sent_total":`)
		assert.Contains(t, body, `"ticks_total":`)
	})

	t.Run("logs one structured line per request that matters, and none for routine polling", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		f.request(t, http.MethodGet, "/api/state", "")
		f.atCharger(t, validFastChargerID, "plug-in", "")
		f.atCharger(t, validFastChargerID, "plug-in", "")

		// Then
		logged := f.out.String()
		assert.Contains(t, logged, `"msg":"request","method":"POST","path":"/api/chargers/EVSE-000002/actions/plug-in","status":200`)
		assert.Contains(t, logged, `"path":"/api/chargers/EVSE-000002/actions/plug-in","status":409`)
		assert.Contains(t, logged, `"world_id":"WORLD-000001"`)
		pollLogged := strings.Contains(logged, `"path":"/api/state"`)
		assert.Equal(t, pollLogged, false)
	})
}
