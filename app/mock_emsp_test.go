package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cposim/app"
	"cposim/assert"
	"cposim/gateway/random"
	"cposim/gateway/scheduler"
	"cposim/mockemsp"
)

const (
	awaitPollFrequency = 5 * time.Millisecond
	awaitTimeout       = time.Second
)

func newMockEMSPFixture(t *testing.T) fixture {
	t.Helper()

	return newMockEMSPFixtureWithRandom(t, luckyRandomGateway())
}

func newMockEMSPFixtureWithRandom(t *testing.T, randomGateway random.Gateway) fixture {
	t.Helper()

	out := &safeBuffer{}
	fakeTicker := scheduler.NewFakeTicker()
	clock := &wallClock{now: time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)}

	// the simulator needs its own URL before it can be built, so claim the port first
	server := httptest.NewUnstartedServer(nil)
	selfBaseURL := "http://" + server.Listener.Addr().String()

	simulator, err := app.NewSimulatorWithTicker(
		app.Config{
			EMSPBaseURL:         selfBaseURL + mockemsp.ReceiverPath,
			MockEMSPSelfBaseURL: selfBaseURL,
			Out:                 out,
		},
		func() scheduler.Ticker { return fakeTicker },
		randomGateway,
		clock.Now,
	)
	assert.NoError(t, err)

	server.Config.Handler = simulator.Handler()
	server.Start()
	assert.NoError(t, simulator.Start())
	assert.NoError(t, simulator.Seed())
	assert.NoError(t, simulator.ConnectMockEMSP())

	t.Cleanup(func() {
		assert.NoError(t, simulator.Stop())
		server.Close()
	})

	return fixture{
		fakeTicker: fakeTicker,
		out:        out,
		server:     server,
		wallClock:  clock,
	}
}

// awaitBelief polls what the mock eMSP believes until it satisfies the condition. Pushes reach
// the mock over real HTTP from another goroutine, so there is no hook to wait on.
func (f fixture) awaitBelief(t *testing.T, description string, condition func(mockemsp.State) bool) (mockemsp.State, error) {
	t.Helper()

	poll := time.NewTicker(awaitPollFrequency)
	defer poll.Stop()
	deadline := time.After(awaitTimeout)

	for {
		_, body := f.request(t, http.MethodGet, "/emsp/api/state", "")

		var believed mockemsp.State
		assert.NoError(t, json.Unmarshal([]byte(body), &believed))

		if condition(believed) {
			return believed, nil
		}

		select {
		case <-poll.C:
		case <-deadline:
			return mockemsp.State{}, fmt.Errorf("mock eMSP never believed that %s within %v; it believes: %s", description, awaitTimeout, body)
		}
	}
}

func evseStatus(believed mockemsp.State, evseUID string) string {
	for _, location := range believed.Locations {
		for _, evse := range location.EVSEs {
			if evse.UID == evseUID {
				return evse.Status
			}
		}
	}

	return ""
}

func TestSimulatorWithMockEMSP(t *testing.T) {
	t.Run("lets the driver's app start and stop a charge, and ends with a bill", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)
		_, err := f.awaitBelief(t, "the fast charger is AVAILABLE", func(believed mockemsp.State) bool {
			return evseStatus(believed, validFastChargerID) == "AVAILABLE"
		})
		assert.NoError(t, err)
		f.request(t, http.MethodPost, "/api/chargers/"+validFastChargerID+"/actions/plug-in", "")

		// When
		statusCode, body := f.request(t, http.MethodPost, "/emsp/api/start", `{"location_id": "`+validHubSiteID+`", "evse_uid": "`+validFastChargerID+`"}`)
		f.advance(t, 2*time.Second)

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		assert.Contains(t, body, `"response":"ACCEPTED"`)
		believed, err := f.awaitBelief(t, "the start was ACCEPTED and the charger is CHARGING", func(believed mockemsp.State) bool {
			return believed.Commands[0].Result == "ACCEPTED" && evseStatus(believed, validFastChargerID) == "CHARGING" && len(believed.Sessions) == 1
		})
		assert.NoError(t, err)
		assert.Equal(t, believed.Sessions[0].CDRToken.UID, "DRIVER-1")

		// When
		f.advance(t, 6*time.Minute)
		statusCode, _ = f.request(t, http.MethodPost, "/emsp/api/stop", `{"session_id": "`+believed.Sessions[0].ID+`"}`)
		f.advance(t, 2*time.Second)

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		believed, err = f.awaitBelief(t, "the session is COMPLETED and billed", func(believed mockemsp.State) bool {
			return believed.Sessions[0].Status == "COMPLETED" && len(believed.CDRs) == 1
		})
		assert.NoError(t, err)
		// 6 min + the stop command's 2 s latency at 50 kW and 0.45/kWh
		assert.Equal(t, believed.CDRs[0].TotalCost.ExclVAT, 2.26)
		assert.Equal(t, believed.Commands[1].Result, "ACCEPTED")
	})

	t.Run("shows the driver's app a TIMEOUT when nobody plugs in", func(t *testing.T) {
		// Given
		f := newMockEMSPFixture(t)

		// When
		f.request(t, http.MethodPost, "/emsp/api/start", `{"location_id": "`+validHubSiteID+`", "evse_uid": "`+validFastChargerID+`"}`)
		f.advance(t, 60*time.Second)

		// Then
		believed, err := f.awaitBelief(t, "the start timed out", func(believed mockemsp.State) bool {
			return believed.Commands[0].Result == "TIMEOUT"
		})
		assert.NoError(t, err)
		sessionCount := len(believed.Sessions)
		assert.Equal(t, sessionCount, 0)
	})
}
