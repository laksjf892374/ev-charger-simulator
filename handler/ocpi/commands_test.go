package ocpi_test

import (
	"errors"
	"net/http"
	"testing"

	"cposim/controller/command"
	"cposim/entity"
	"cposim/internal/assert"
	"cposim/ocpi"
)

func TestCommands(t *testing.T) {
	validStartSessionBody := `{
		"response_url": "` + validResponseURL + `",
		"token": {"uid": "TOKEN-1", "type": "APP_USER", "contract_id": "US-EMS-C1"},
		"location_id": "` + validSiteID + `",
		"evse_uid": "` + validChargerID + `",
		"authorization_reference": "AUTH-1"
	}`

	t.Run("answers NOT_SUPPORTED for a command the CPO does not implement", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/RESERVE_NOW", `{}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded).Result, "NOT_SUPPORTED")
	})

	t.Run("rejects a body that is not JSON", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", `not json`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
		assert.Equal(t, decoded.StatusCode, 2001)
	})

	t.Run("rejects a response_url that is not http(s)", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "file:///etc/passwd", "session_id": "SES-1"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
		stopSessionCallCount := len(f.commandController.StopSessionCalledWith)
		assert.Equal(t, stopSessionCallCount, 0)
	})

	t.Run("answers unknown location when the EVSE does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerErr = errors.New("boom")

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Equal(t, decoded.StatusCode, 2003)
	})

	t.Run("answers REJECTED with the reason when the command controller rejects the start", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{
			Message: "charger refused the request",
			State:   entity.CommandStateRejected,
		}

		// When
		_, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		response := decodeData[ocpi.CommandResponse](t, decoded)
		assert.Equal(t, response.Result, "REJECTED")
		assert.Equal(t, response.Message[0].Text, "charger refused the request")
	})

	t.Run("answers UNKNOWN_SESSION when asked to stop a session that does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.sessionController.GetSessionErr = errors.New("boom")

		// When
		_, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "`+validResponseURL+`", "session_id": "missing"}`)

		// Then
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded).Result, "UNKNOWN_SESSION")
	})

	t.Run("passes a start to the command controller and answers ACCEPTED with the timeout", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{State: entity.CommandStatePending}

		// When
		recorder, decoded := serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", validStartSessionBody)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, decodeData[ocpi.CommandResponse](t, decoded), ocpi.CommandResponse{Result: "ACCEPTED", Timeout: 90})
		assert.Equal(t, f.commandController.StartSessionCalledWith, []command.StartSessionInput{{
			AuthorizationReference: "AUTH-1",
			CallbackReference:      validResponseURL,
			ChargerID:              validChargerID,
			Token:                  entity.Token{ContractID: "US-EMS-C1", Type: "APP_USER", UID: "TOKEN-1"},
		}})
	})

	t.Run("passes a stop and an unlock to the command controller", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/STOP_SESSION", `{"response_url": "`+validResponseURL+`", "session_id": "SES-1"}`)
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/UNLOCK_CONNECTOR", `{"response_url": "`+validResponseURL+`", "location_id": "`+validSiteID+`", "evse_uid": "`+validChargerID+`", "connector_id": "1"}`)

		// Then
		assert.Equal(t, f.commandController.StopSessionCalledWith, []command.StopSessionInput{{
			CallbackReference: validResponseURL,
			SessionID:         "SES-1",
		}})
		assert.Equal(t, f.commandController.UnlockConnectorCalledWith, []command.UnlockConnectorInput{{
			CallbackReference: validResponseURL,
			ChargerID:         validChargerID,
		}})
	})
}
