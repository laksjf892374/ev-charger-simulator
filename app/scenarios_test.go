package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"cposim/assert"
	"cposim/gateway/random"
	"cposim/mockemsp"
)

// Scenarios are whole user stories, driven only through the public HTTP APIs of a running
// simulator with the mock eMSP mounted: what a person at a charger, a driver with a phone, and an
// operator would actually do, and what each of them should see.

const (
	almostFullVehicle    = `{"battery_capacity_kwh": 60, "max_power_kw": 150, "state_of_charge": 0.95}`
	defaultVehicle       = ``
	validHubChargerID    = "EVSE-000001" // seeded, 150 kW
	validSecondChargerID = validFastChargerID
)

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

func TestErrorScenarios(t *testing.T) {
	t.Run("a charger breaks mid-session: partial bill, out of order, cable stuck until the phone unlocks it", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.configure(t, validFastChargerID, `[{"kind": "fault_mid_session", "params": {"after_s": 360}}]`)
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)

		// When
		f.advance(t, 6*time.Minute)

		// Then
		believed := f.phoneSees(t, "the session ended early and the charger is out of order", func(believed mockemsp.State) bool {
			return len(believed.CDRs) == 1 && evseStatus(believed, validFastChargerID) == "OUTOFORDER"
		})
		assert.Equal(t, believed.CDRs[0].TotalEnergy, 5.0)
		assert.Equal(t, f.world(t).Sessions[0].StopReason, "FAULT")
		assert.Equal(t, f.atCharger(t, validFastChargerID, "unplug", ""), http.StatusConflict)

		// When
		statusCode, _ := f.request(t, http.MethodPost, "/emsp/api/unlock", `{"location_id": "`+validHubSiteID+`", "evse_uid": "`+validFastChargerID+`"}`)
		f.advance(t, 2*time.Second)

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		f.phoneSees(t, "the unlock worked", func(believed mockemsp.State) bool {
			return believed.Commands[len(believed.Commands)-1].Result == "ACCEPTED"
		})
		assert.Equal(t, f.atCharger(t, validFastChargerID, "unplug", ""), http.StatusOK)

		// When
		assert.Equal(t, f.atCharger(t, validFastChargerID, "clear-fault", ""), http.StatusOK)

		// Then
		f.phoneSees(t, "the repaired charger is available again", func(believed mockemsp.State) bool {
			return evseStatus(believed, validFastChargerID) == "AVAILABLE"
		})
	})

	t.Run("a driver tries a charger that is out of order", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "inject-fault", ""), http.StatusOK)

		// When
		command := f.phoneStart(t, validHubSiteID, validFastChargerID)
		f.advance(t, 2*time.Second)

		// Then
		believed := f.phoneSees(t, "the start was refused because the charger is broken", func(believed mockemsp.State) bool {
			return commandResult(believed, command.UID) == "EVSE_INOPERATIVE"
		})
		sessionCount := len(believed.Sessions)
		assert.Equal(t, sessionCount, 0)
	})

	t.Run("a second driver tries a charger that is already in use", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.startCharging(t, validHubSiteID, validFastChargerID, defaultVehicle)

		// When
		command := f.phoneStart(t, validHubSiteID, validFastChargerID)
		f.advance(t, 2*time.Second)

		// Then
		believed := f.phoneSees(t, "the second start was refused because the charger is busy", func(believed mockemsp.State) bool {
			return commandResult(believed, command.UID) == "EVSE_OCCUPIED"
		})
		sessionCount := len(believed.Sessions)
		assert.Equal(t, sessionCount, 1)
	})

	t.Run("a charger refuses start requests outright", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		f.configure(t, validFastChargerID, `[{"kind": "reject_start"}]`)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "plug-in", defaultVehicle), http.StatusOK)

		// When
		command := f.phoneStart(t, validHubSiteID, validFastChargerID)

		// Then
		assert.Equal(t, command.Response, "REJECTED")
		assert.Equal(t, command.Result, "REJECTED")
		assert.Equal(t, command.Message, "charger refused the request")
	})

	t.Run("an everyday charger fails a start at random, and the phone is told why", func(t *testing.T) {
		// Given
		unluckyRandomGateway := random.NewFakeGateway()
		unluckyRandomGateway.Float64Result = 0.01
		f := newMockEMSPFixtureWithRandom(t, unluckyRandomGateway)
		assert.Equal(t, f.atCharger(t, validFastChargerID, "plug-in", defaultVehicle), http.StatusOK)

		// When
		command := f.phoneStart(t, validHubSiteID, validFastChargerID)
		f.advance(t, 2*time.Second)

		// Then
		believed := f.phoneSees(t, "the start failed", func(believed mockemsp.State) bool {
			return commandResult(believed, command.UID) == "FAILED"
		})
		assert.Contains(t, believed.Commands[0].Message, "random failure")
		assert.Equal(t, f.charger(t, validFastChargerID).State, "PREPARING")
	})

	t.Run("the phone asks to stop a session that does not exist", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		command := f.phoneStop(t, "SES-999999")

		// Then
		assert.Equal(t, command.Response, "UNKNOWN_SESSION")
		assert.Equal(t, command.Result, "UNKNOWN_SESSION")
	})

	t.Run("the phone asks to start a charger that does not exist", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		command := f.phoneStart(t, validHubSiteID, "NOPE")

		// Then
		assert.Equal(t, command.Response, mockemsp.CommandResponseRefused)
		assert.Equal(t, command.Result, mockemsp.CommandResponseRefused)
		assert.Contains(t, command.Message, "OCPI status 2003")
	})
}

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
