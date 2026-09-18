package app_test

import (
	"net/http"
	"testing"
	"time"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
)

// Scenarios are whole user stories, driven only through the public HTTP APIs of a running
// simulator with the mock eMSP mounted: what a person at a charger, a driver with a phone, and an
// operator would actually do, and what each of them should see.

func TestHappyPathScenarios(t *testing.T) {
	t.Run("a driver plugs in, starts from the phone, charges until the battery is full, and unplugs", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		sessionID := f.startCharging(t, validHubSiteID, validFastChargerID, almostFullVehicle)

		// When
		for i := 0; i < 8; i++ {
			f.advance(t, 5*time.Minute)
		}

		// Then
		believed := f.phoneSees(t, "the session completed by itself and was billed", func(believed mockemsp.State) bool {
			return len(believed.CDRs) == 1 && believed.Sessions[0].Status == "COMPLETED" && evseStatus(believed, validFastChargerID) == "BLOCKED"
		})
		assert.Equal(t, believed.CDRs[0].SessionID, sessionID)
		assert.Equal(t, believed.CDRs[0].TotalEnergy, 3.0)
		assert.Equal(t, believed.CDRs[0].TotalCost.ExclVAT, 1.35)
		finishedCharger := f.charger(t, validFastChargerID)
		assert.Equal(t, finishedCharger.State, "FINISHING")
		assert.Equal(t, *finishedCharger.LiveStateOfCharge, 1.0)
		assert.Equal(t, f.world(t).Sessions[0].StopReason, "VEHICLE_FULL")

		// When
		statusCode := f.atCharger(t, validFastChargerID, "unplug", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		f.phoneSees(t, "the charger is free again", func(believed mockemsp.State) bool {
			return evseStatus(believed, validFastChargerID) == "AVAILABLE"
		})
	})

	t.Run("a driver starts from the phone first and plugs in before the start times out", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		command := f.phoneStart(t, validHubSiteID, validFastChargerID)
		f.advance(t, 30*time.Second)

		// Then
		assert.Equal(t, command.Response, "ACCEPTED")
		assert.Equal(t, commandResult(f.phoneSees(t, "nothing has happened yet", func(mockemsp.State) bool { return true }), command.UID), "PENDING")

		// When
		assert.Equal(t, f.atCharger(t, validFastChargerID, "plug-in", defaultVehicle), http.StatusOK)
		f.advance(t, time.Second)

		// Then
		f.phoneSees(t, "the waiting start went through once the car was plugged in", func(believed mockemsp.State) bool {
			return commandResult(believed, command.UID) == "ACCEPTED" && evseStatus(believed, validFastChargerID) == "CHARGING"
		})
	})

	t.Run("a driver stops with the button on the charger, and the phone still gets the bill", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)
		f.advance(t, 12*time.Minute)

		// When
		statusCode := f.atCharger(t, validFastChargerID, "press-stop", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		believed := f.phoneSees(t, "the phone heard about a stop it did not ask for", func(believed mockemsp.State) bool {
			return len(believed.CDRs) == 1 && believed.Sessions[0].Status == "COMPLETED"
		})
		assert.Equal(t, believed.CDRs[0].TotalEnergy, 10.0)
		assert.Equal(t, f.world(t).Sessions[0].StopReason, "STOP_BUTTON")
	})

	t.Run("two drivers charge side by side and are billed separately at each charger's own price", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		hubSessionID := f.startCharging(t, validHubSiteID, validHubChargerID, defaultVehicle)
		fastSessionID := f.startCharging(t, validHubSiteID, validSecondChargerID, defaultVehicle)
		f.advance(t, 6*time.Minute)

		// When
		f.phoneStop(t, hubSessionID)
		f.phoneStop(t, fastSessionID)
		f.advance(t, 2*time.Second)

		// Then
		believed := f.phoneSees(t, "both sessions were billed", func(believed mockemsp.State) bool {
			return len(believed.CDRs) == 2
		})
		costBySessionID := map[string]float64{}
		for _, cdr := range believed.CDRs {
			costBySessionID[cdr.SessionID] = cdr.TotalCost.ExclVAT
		}
		// 150 kW at 0.55/kWh against 50 kW at 0.45/kWh, for the same six-odd minutes
		assert.Equal(t, costBySessionID[hubSessionID] > 3*costBySessionID[fastSessionID], true)
	})

	t.Run("an operator adds a charger that drivers can see and use, then takes it away again", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		statusCode, body := f.request(t, http.MethodPost, "/api/chargers", `{"charger_id": "NEW-1", "site_id": "`+validHubSiteID+`", "max_power_kw": 22}`)

		// Then
		assert.Equal(t, statusCode, http.StatusCreated)
		assert.Contains(t, body, `"kind":"realistic_reliability"`)
		f.phoneSees(t, "the new charger showed up on the phone", func(believed mockemsp.State) bool {
			return evseStatus(believed, "NEW-1") == "AVAILABLE"
		})
		f.startCharging(t, validHubSiteID, "NEW-1", defaultVehicle)

		// When
		statusCode, _ = f.request(t, http.MethodDelete, "/api/chargers/NEW-1", "")

		// Then
		assert.Equal(t, statusCode, http.StatusConflict)

		// When
		assert.Equal(t, f.atCharger(t, "NEW-1", "press-stop", ""), http.StatusOK)
		assert.Equal(t, f.atCharger(t, "NEW-1", "unplug", ""), http.StatusOK)
		statusCode, _ = f.request(t, http.MethodDelete, "/api/chargers/NEW-1", "")

		// Then
		assert.Equal(t, statusCode, http.StatusNoContent)
		f.phoneSees(t, "the removed charger disappeared from the phone", func(believed mockemsp.State) bool {
			return evseStatus(believed, "NEW-1") == ""
		})
	})

	t.Run("an eMSP pulling over OCPI sees the same sessions and bills it was pushed", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		sessionID := f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)
		f.advance(t, 6*time.Minute)
		f.phoneStop(t, sessionID)
		f.advance(t, 2*time.Second)
		believed := f.phoneSees(t, "the session was billed", func(believed mockemsp.State) bool { return len(believed.CDRs) == 1 })

		// When
		_, sessionsBody := f.request(t, http.MethodGet, "/ocpi/cpo/2.2.1/sessions", "")
		_, cdrsBody := f.request(t, http.MethodGet, "/ocpi/cpo/2.2.1/cdrs", "")

		// Then
		assert.Contains(t, sessionsBody, `"id":"`+sessionID+`"`)
		assert.Contains(t, sessionsBody, `"status":"COMPLETED"`)
		assert.Contains(t, cdrsBody, `"id":"`+believed.CDRs[0].ID+`"`)
		assert.Contains(t, cdrsBody, `"session_id":"`+sessionID+`"`)
	})
}
