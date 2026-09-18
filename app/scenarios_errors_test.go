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
