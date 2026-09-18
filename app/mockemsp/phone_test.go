package mockemsp_test

import (
	"errors"
	"net/http"
	"testing"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
	"cposim/ocpi"
)

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
