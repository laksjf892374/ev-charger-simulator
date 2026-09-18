package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cposim/app"
	"cposim/assert"
	"cposim/gateway/random"
	"cposim/gateway/scheduler"
)

const (
	validFastChargerID  = "EVSE-000002" // seeded, 50 kW, well-behaved
	validFlakyChargerID = "EVSE-000003" // seeded with start_fails after 8 s
	validFlakySiteID    = "SITE-000002"
	validHubSiteID      = "SITE-000001"
)

// luckyRandomGateway keeps the seeded, realistically reliable chargers from failing at random.
func luckyRandomGateway() *random.FakeGateway {
	randomGateway := random.NewFakeGateway()
	randomGateway.Float64Result = 0.999

	return randomGateway
}

type wallClock struct {
	now time.Time
	mu  sync.Mutex
}

func (c *wallClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *wallClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(duration)
}

// safeBuffer is an io.Writer a test can read while the simulator's goroutines are still writing.
type safeBuffer struct {
	buffer bytes.Buffer
	mu     sync.Mutex
}

func (b *safeBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(data)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

type fixture struct {
	fakeEMSP   *app.FakeEMSP
	fakeTicker *scheduler.FakeTicker
	out        *safeBuffer
	server     *httptest.Server
	wallClock  *wallClock
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	fakeEMSP := app.NewFakeEMSP()
	fakeTicker := scheduler.NewFakeTicker()
	clock := &wallClock{now: time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)}

	simulator, err := app.NewSimulatorWithTicker(
		app.Config{EMSPBaseURL: fakeEMSP.URL + "/ocpi/2.2.1", Out: &bytes.Buffer{}},
		func() scheduler.Ticker { return fakeTicker },
		luckyRandomGateway(),
		clock.Now,
	)
	assert.NoError(t, err)

	server := httptest.NewServer(simulator.Handler())
	assert.NoError(t, simulator.Start())
	assert.NoError(t, simulator.Seed())

	t.Cleanup(func() {
		assert.NoError(t, simulator.Stop())
		server.Close()
		fakeEMSP.Close()
	})

	return fixture{
		fakeEMSP:   fakeEMSP,
		fakeTicker: fakeTicker,
		server:     server,
		wallClock:  clock,
	}
}

// advance moves time forward and runs one full simulation tick. The second Tick only returns
// once the job is ready for another tick, i.e. once the first tick's callback has finished.
func (f fixture) advance(t *testing.T, duration time.Duration) {
	t.Helper()

	f.wallClock.Advance(duration)
	assert.NoError(t, f.fakeTicker.Tick())
	assert.NoError(t, f.fakeTicker.Tick())
}

func (f fixture) request(t *testing.T, method string, path string, body string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	assert.NoError(t, err)

	response, err := http.DefaultClient.Do(request)
	assert.NoError(t, err)
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	assert.NoError(t, err)

	return response.StatusCode, string(responseBody)
}

func (f fixture) startSessionBody(siteID string, chargerID string) string {
	body, _ := json.Marshal(map[string]any{
		"evse_uid":     chargerID,
		"location_id":  siteID,
		"response_url": f.fakeEMSP.URL + "/ocpi/2.2.1/commands/START_SESSION/1",
		"token":        map[string]string{"uid": "DRIVER-1", "type": "APP_USER", "contract_id": "US-EMS-C0001"},
	})

	return string(body)
}

func TestSimulator(t *testing.T) {
	t.Run("announces the seeded world to the eMSP and serves it over OCPI", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		statusCode, body := f.request(t, http.MethodGet, "/ocpi/cpo/2.2.1/locations", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		assert.Contains(t, body, "Downtown Fast Charging Hub")
		assert.Contains(t, body, validFastChargerID)
		_, err := f.fakeEMSP.AwaitRequest("/locations/US/SIM/"+validHubSiteID, "Downtown Fast Charging Hub")
		assert.NoError(t, err)
		_, err = f.fakeEMSP.AwaitRequest("/locations/US/SIM/"+validFlakySiteID+"/"+validFlakyChargerID, `"status":"AVAILABLE"`)
		assert.NoError(t, err)
	})

	t.Run("runs a remote-started session end to end: command result, session updates, CDR", func(t *testing.T) {
		// Given
		f := newFixture(t)
		statusCode, _ := f.request(t, http.MethodPost, "/api/chargers/"+validFastChargerID+"/actions/plug-in", "")
		assert.Equal(t, statusCode, http.StatusOK)

		// When
		statusCode, body := f.request(t, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", f.startSessionBody(validHubSiteID, validFastChargerID))

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		assert.Contains(t, body, `"result":"ACCEPTED"`)

		// When
		f.advance(t, 2*time.Second)

		// Then
		_, err := f.fakeEMSP.AwaitRequest("/commands/START_SESSION/1", `"result":"ACCEPTED"`)
		assert.NoError(t, err)
		_, err = f.fakeEMSP.AwaitRequest("/sessions/US/SIM/SES-000001", `"status":"ACTIVE"`)
		assert.NoError(t, err)
		_, err = f.fakeEMSP.AwaitRequest("/"+validFastChargerID, `"status":"CHARGING"`)
		assert.NoError(t, err)

		// When
		f.advance(t, 6*time.Minute)

		// Then
		_, err = f.fakeEMSP.AwaitRequest("/sessions/US/SIM/SES-000001", `"kwh":5,`)
		assert.NoError(t, err)

		// When
		statusCode, _ = f.request(t, http.MethodPost, "/api/chargers/"+validFastChargerID+"/actions/press-stop", "")

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		_, err = f.fakeEMSP.AwaitRequest("/sessions/US/SIM/SES-000001", `"status":"COMPLETED"`)
		assert.NoError(t, err)
		cdrRequest, err := f.fakeEMSP.AwaitRequest("/cdrs", `"session_id":"SES-000001"`)
		assert.NoError(t, err)
		assert.Contains(t, cdrRequest.Body, `"total_energy":5,`)
		assert.Contains(t, cdrRequest.Body, `"total_cost":{"excl_vat":2.25}`)
		_, err = f.fakeEMSP.AwaitRequest("/"+validFastChargerID, `"status":"BLOCKED"`)
		assert.NoError(t, err)
	})

	t.Run("reports FAILED for a charger configured to fail its starts, and starts no session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		statusCode, _ := f.request(t, http.MethodPost, "/api/chargers/"+validFlakyChargerID+"/actions/plug-in", "")
		assert.Equal(t, statusCode, http.StatusOK)

		// When
		_, body := f.request(t, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", f.startSessionBody(validFlakySiteID, validFlakyChargerID))
		f.advance(t, 8*time.Second)

		// Then
		assert.Contains(t, body, `"result":"ACCEPTED"`)
		_, err := f.fakeEMSP.AwaitRequest("/commands/START_SESSION/1", `"result":"FAILED"`)
		assert.NoError(t, err)
		_, sessionsBody := f.request(t, http.MethodGet, "/api/sessions", "")
		assert.Equal(t, strings.TrimSpace(sessionsBody), "[]")
	})

	t.Run("runs simulated time faster when the clock speed is raised through the API", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.request(t, http.MethodPost, "/api/chargers/"+validFastChargerID+"/actions/plug-in", "")
		f.request(t, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", f.startSessionBody(validHubSiteID, validFastChargerID))
		f.advance(t, 2*time.Second)

		// When
		statusCode, _ := f.request(t, http.MethodPut, "/api/clock", `{"speed": 60}`)
		f.advance(t, 6*time.Second)

		// Then
		assert.Equal(t, statusCode, http.StatusOK)
		_, err := f.fakeEMSP.AwaitRequest("/sessions/US/SIM/SES-000001", `"kwh":5,`)
		assert.NoError(t, err)
	})
}
