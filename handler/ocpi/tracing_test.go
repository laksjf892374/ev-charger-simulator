package ocpi_test

import (
	"net/http"
	"testing"

	"cposim/entity"
	"cposim/gateway/trace"
	"cposim/internal/assert"
)

func TestTracing(t *testing.T) {
	t.Run("records the exchange with a plain-English summary of request and answer", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.commandController.StartSessionResult = entity.Command{State: entity.CommandStatePending}
		body := `{"response_url": "` + validResponseURL + `", "token": {"uid": "TOKEN-1"}, "location_id": "` + validSiteID + `", "evse_uid": "` + validChargerID + `"}`

		// When
		serve(t, f, http.MethodPost, "/ocpi/cpo/2.2.1/commands/START_SESSION", body)

		// Then
		entries := f.traceGateway.Recorded()
		entryCount := len(entries)
		assert.Equal(t, entryCount, 1)
		assert.Equal(t, entries[0].Direction, trace.DirectionInbound)
		assert.Equal(t, entries[0].Method, http.MethodPost)
		assert.Equal(t, entries[0].Module, trace.ModuleCommands)
		assert.Equal(t, entries[0].URL, "/ocpi/cpo/2.2.1/commands/START_SESSION")
		assert.Equal(t, entries[0].StatusCode, http.StatusOK)
		assert.Equal(t, entries[0].RequestBody, body)
		assert.Contains(t, entries[0].ResponseBody, `"result":"ACCEPTED"`)
		assert.Equal(t, entries[0].Summary, "eMSP asks the CPO to start charging on EVSE-000001 for driver token TOKEN-1. CPO answers ACCEPTED: the real outcome will follow as a separate call")
	})

	t.Run("records calls to endpoints that do not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder, _ := serve(t, f, http.MethodGet, "/ocpi/cpo/2.2.1/tariffs", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Contains(t, f.traceGateway.Recorded()[0].Summary, "does not have")
	})
}
