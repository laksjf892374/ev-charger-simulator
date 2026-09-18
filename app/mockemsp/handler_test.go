package mockemsp_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
	"cposim/ocpi"
)

var validConfig = mockemsp.Config{
	DriverToken: ocpi.Token{ContractID: "US-EMS-C0001", Type: "APP_USER", UID: "DRIVER-1"},
	SelfBaseURL: "http://emsp.example/",
}

type fixture struct {
	cpoClient *mockemsp.FakeCPOClient
	mock      mockemsp.Mock
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	cpoClient := mockemsp.NewFakeCPOClient()
	cpoClient.SendCommandResult = ocpi.CommandResponse{Result: "ACCEPTED"}

	return fixture{
		cpoClient: cpoClient,
		mock:      mockemsp.NewMock(validConfig, cpoClient),
	}
}

func serve(t *testing.T, f fixture, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	f.mock.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))

	return recorder
}

func state(t *testing.T, f fixture) mockemsp.State {
	t.Helper()

	recorder := serve(t, f, http.MethodGet, "/emsp/api/state", "")
	assert.Equal(t, recorder.Code, http.StatusOK)

	var decoded mockemsp.State
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &decoded))

	return decoded
}

func TestReceiver(t *testing.T) {
	t.Run("rejects a push that is not JSON", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", "not json")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
	})

	t.Run("answers not found for the result of a command it never sent", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/commands/START_SESSION/REQ-9999", `{"result": "ACCEPTED"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
	})

	t.Run("keeps a location's EVSEs when the location is announced again without them", func(t *testing.T) {
		// Given
		f := newFixture(t)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "AVAILABLE"}`)

		// When
		recorder := serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1", `{"name": "Oakland Hub", "evses": []}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		locations := state(t, f).Locations
		assert.Equal(t, locations[0].Name, "Oakland Hub")
		assert.Equal(t, locations[0].EVSEs[0].UID, "EVSE-1")
	})

	t.Run("believes the latest EVSE status and forgets an EVSE that is REMOVED", func(t *testing.T) {
		// Given
		f := newFixture(t)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "AVAILABLE"}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-2", `{"status": "AVAILABLE"}`)

		// When
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "CHARGING"}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-2", `{"status": "REMOVED"}`)

		// Then
		evses := state(t, f).Locations[0].EVSEs
		evseCount := len(evses)
		assert.Equal(t, evseCount, 1)
		assert.Equal(t, evses[0].Status, "CHARGING")
	})

	t.Run("stores sessions by ID and bills every CDR it is sent, even a duplicate", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/sessions/US/SIM/SES-1", `{"status": "ACTIVE", "kwh": 1}`)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/sessions/US/SIM/SES-1", `{"status": "COMPLETED", "kwh": 5}`)
		serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-1"}`)
		serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-1"}`)

		// Then
		believed := state(t, f)
		sessionCount := len(believed.Sessions)
		assert.Equal(t, sessionCount, 1)
		assert.Equal(t, believed.Sessions[0].Status, "COMPLETED")
		assert.Equal(t, believed.Sessions[0].KWH, 5.0)
		cdrCount := len(believed.CDRs)
		assert.Equal(t, cdrCount, 2)
	})
}

func TestHistoryIsBounded(t *testing.T) {
	t.Run("keeps only the most recent bills", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		for i := 0; i < 205; i++ {
			serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/cdrs", `{"id": "CDR-`+strconv.Itoa(i)+`"}`)
		}

		// Then
		cdrs := state(t, f).CDRs
		cdrCount := len(cdrs)
		assert.Equal(t, cdrCount, 200)
		assert.Equal(t, cdrs[0].ID, "CDR-5")
	})
}

func TestStart(t *testing.T) {
	t.Run("answers bad gateway and records the error when the CPO cannot be reached", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.cpoClient.SendCommandErr = errors.New("boom")

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/api/start", `{"location_id": "SITE-1", "evse_uid": "EVSE-1"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadGateway)
		assert.Equal(t, state(t, f).Commands[0].Response, "ERROR")
	})

	t.Run("treats a REJECTED answer as the final outcome", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.cpoClient.SendCommandResult = ocpi.CommandResponse{
			Message: []ocpi.DisplayText{{Language: "en", Text: "charger refused the request"}},
			Result:  "REJECTED",
		}

		// When
		serve(t, f, http.MethodPost, "/emsp/api/start", `{"location_id": "SITE-1", "evse_uid": "EVSE-1"}`)

		// Then
		assert.Equal(t, state(t, f).Commands, []mockemsp.Command{{
			EVSEUID:  "EVSE-1",
			Kind:     "START_SESSION",
			Message:  "charger refused the request",
			Response: "REJECTED",
			Result:   "REJECTED",
			UID:      "REQ-0001",
		}})
	})

	t.Run("sends START_SESSION with the driver's token and a response_url back to itself, then waits for the result", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/api/start", `{"location_id": "SITE-1", "evse_uid": "EVSE-1"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, f.cpoClient.SendCommandCalls, []mockemsp.SendCommandCall{{
			Kind: "START_SESSION",
			Request: ocpi.StartSession{
				EVSEUID:     "EVSE-1",
				LocationID:  "SITE-1",
				ResponseURL: "http://emsp.example/emsp/ocpi/2.2.1/commands/START_SESSION/REQ-0001",
				Token:       validConfig.DriverToken,
			},
		}})
		assert.Equal(t, state(t, f).Commands[0].Response, "ACCEPTED")
		assert.Equal(t, state(t, f).Commands[0].Result, mockemsp.CommandResultPending)

		// When
		recorder = serve(t, f, http.MethodPost, "/emsp/ocpi/2.2.1/commands/START_SESSION/REQ-0001", `{"result": "TIMEOUT", "message": [{"language": "en", "text": "no vehicle was plugged in"}]}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, state(t, f).Commands[0].Result, "TIMEOUT")
		assert.Equal(t, state(t, f).Commands[0].Message, "no vehicle was plugged in")
	})
}

func TestStopAndUnlock(t *testing.T) {
	t.Run("sends STOP_SESSION and UNLOCK_CONNECTOR with their own response URLs", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		serve(t, f, http.MethodPost, "/emsp/api/stop", `{"session_id": "SES-1"}`)
		serve(t, f, http.MethodPost, "/emsp/api/unlock", `{"location_id": "SITE-1", "evse_uid": "EVSE-1"}`)

		// Then
		assert.Equal(t, f.cpoClient.SendCommandCalls, []mockemsp.SendCommandCall{
			{
				Kind: "STOP_SESSION",
				Request: ocpi.StopSession{
					ResponseURL: "http://emsp.example/emsp/ocpi/2.2.1/commands/STOP_SESSION/REQ-0001",
					SessionID:   "SES-1",
				},
			},
			{
				Kind: "UNLOCK_CONNECTOR",
				Request: ocpi.UnlockConnector{
					ConnectorID: "1",
					EVSEUID:     "EVSE-1",
					LocationID:  "SITE-1",
					ResponseURL: "http://emsp.example/emsp/ocpi/2.2.1/commands/UNLOCK_CONNECTOR/REQ-0002",
				},
			},
		})
	})
}

func TestSync(t *testing.T) {
	t.Run("answers bad gateway when locations cannot be pulled", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.cpoClient.PullLocationsErr = errors.New("boom")

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/api/sync", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadGateway)
	})

	t.Run("replaces what it believes with what the CPO says now", func(t *testing.T) {
		// Given
		f := newFixture(t)
		serve(t, f, http.MethodPut, "/emsp/ocpi/2.2.1/locations/US/SIM/SITE-1/EVSE-1", `{"status": "CHARGING"}`)
		f.cpoClient.PullLocationsResult = []ocpi.Location{{
			EVSEs: []ocpi.EVSE{{Status: "AVAILABLE", UID: "EVSE-1"}},
			ID:    "SITE-1",
		}}

		// When
		recorder := serve(t, f, http.MethodPost, "/emsp/api/sync", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, state(t, f).Locations[0].EVSEs[0].Status, "AVAILABLE")
	})
}
