package app_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"cposim/assert"
	"cposim/mockemsp"
)

// The helpers below let a scenario read like the story it tells. They only ever use the public
// HTTP APIs: /api is the person at the charger (or the operator), /emsp/api is the driver's phone.

type chargerView struct {
	ActiveSession *struct {
		EnergyDeliveredKWH float64 `json:"energy_delivered_kwh"`
		SessionID          string  `json:"session_id"`
	} `json:"active_session"`
	ChargerID         string   `json:"charger_id"`
	ConnectorLocked   bool     `json:"connector_locked"`
	LiveStateOfCharge *float64 `json:"live_state_of_charge"`
	SessionID         string   `json:"session_id"`
	State             string   `json:"state"`
	Vehicle           *struct {
		BatteryCapacityKWH float64 `json:"battery_capacity_kwh"`
		StateOfCharge      float64 `json:"state_of_charge"`
	} `json:"vehicle"`
}

type sessionView struct {
	ChargerID          string  `json:"charger_id"`
	EnergyDeliveredKWH float64 `json:"energy_delivered_kwh"`
	SessionID          string  `json:"session_id"`
	State              string  `json:"state"`
	StopReason         string  `json:"stop_reason"`
}

type cdrView struct {
	CDRID              string  `json:"cdr_id"`
	EnergyDeliveredKWH float64 `json:"energy_delivered_kwh"`
	SessionID          string  `json:"session_id"`
	TotalCost          float64 `json:"total_cost"`
}

type worldView struct {
	CDRs     []cdrView     `json:"cdrs"`
	Chargers []chargerView `json:"chargers"`
	Sessions []sessionView `json:"sessions"`
	WorldID  string        `json:"world_id"`
}

func (f fixture) world(t *testing.T) worldView {
	t.Helper()

	statusCode, body := f.request(t, http.MethodGet, "/api/state", "")
	assert.Equal(t, statusCode, http.StatusOK)

	var view worldView
	assert.NoError(t, json.Unmarshal([]byte(body), &view))

	return view
}

func (f fixture) charger(t *testing.T, chargerID string) chargerView {
	t.Helper()

	for _, view := range f.world(t).Chargers {
		if view.ChargerID == chargerID {
			return view
		}
	}

	t.Fatalf("charger %q does not exist", chargerID)

	return chargerView{}
}

// atCharger performs a physical action and returns the HTTP status, so a scenario can assert
// both that an action worked and that one was refused.
func (f fixture) atCharger(t *testing.T, chargerID string, action string, body string) int {
	t.Helper()

	statusCode, _ := f.request(t, http.MethodPost, "/api/chargers/"+chargerID+"/actions/"+action, body)

	return statusCode
}

func (f fixture) configure(t *testing.T, chargerID string, behaviors string) {
	t.Helper()

	statusCode, body := f.request(t, http.MethodPut, "/api/chargers/"+chargerID+"/behaviors", behaviors)
	if statusCode != http.StatusOK {
		t.Fatalf("configuring %q answered %d: %s", chargerID, statusCode, body)
	}
}

func (f fixture) phoneStart(t *testing.T, siteID string, chargerID string) mockemsp.Command {
	t.Helper()

	statusCode, body := f.request(t, http.MethodPost, "/emsp/api/start", `{"location_id": "`+siteID+`", "evse_uid": "`+chargerID+`"}`)
	assert.Equal(t, statusCode, http.StatusOK)

	var command mockemsp.Command
	assert.NoError(t, json.Unmarshal([]byte(body), &command))

	return command
}

func (f fixture) phoneStop(t *testing.T, sessionID string) mockemsp.Command {
	t.Helper()

	statusCode, body := f.request(t, http.MethodPost, "/emsp/api/stop", `{"session_id": "`+sessionID+`"}`)
	assert.Equal(t, statusCode, http.StatusOK)

	var command mockemsp.Command
	assert.NoError(t, json.Unmarshal([]byte(body), &command))

	return command
}

// phoneSees waits until the phone's view of the world satisfies the condition.
func (f fixture) phoneSees(t *testing.T, description string, condition func(mockemsp.State) bool) mockemsp.State {
	t.Helper()

	believed, err := f.awaitBelief(t, description, condition)
	assert.NoError(t, err)

	return believed
}

func commandResult(believed mockemsp.State, uid string) string {
	for _, command := range believed.Commands {
		if command.UID == uid {
			return command.Result
		}
	}

	return ""
}

// startCharging is the everyday opening of most scenarios: plug in, start from the phone, and
// wait until the phone knows the session is running.
func (f fixture) startCharging(t *testing.T, siteID string, chargerID string, vehicle string) string {
	t.Helper()

	assert.Equal(t, f.atCharger(t, chargerID, "plug-in", vehicle), http.StatusOK)
	command := f.phoneStart(t, siteID, chargerID)
	assert.Equal(t, command.Response, "ACCEPTED")
	f.advance(t, 2*time.Second)

	sessionID := ""
	f.phoneSees(t, "the session on "+chargerID+" is running", func(believed mockemsp.State) bool {
		for _, session := range believed.Sessions {
			if session.EVSEUID == chargerID && session.Status == "ACTIVE" {
				sessionID = session.ID
			}
		}

		return sessionID != "" && commandResult(believed, command.UID) == "ACCEPTED"
	})

	return sessionID
}
