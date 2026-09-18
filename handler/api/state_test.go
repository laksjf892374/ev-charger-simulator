package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"cposim/entity"
	"cposim/gateway/metrics"
	"cposim/gateway/trace"
	"cposim/internal/assert"
)

func TestGetState(t *testing.T) {
	t.Run("rejects a since that is not an integer", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodGet, "/api/state?since=recently", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
	})

	t.Run("returns the whole world, the behavior catalog and the trace after since", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.ListChargersResult = []entity.Charger{{ChargerID: validChargerID}}
		f.traceGateway.ListSinceResult = []trace.Entry{{Sequence: 8, Summary: "something happened"}}

		// When
		recorder := serve(t, f, http.MethodGet, "/api/state?since=7", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, f.traceGateway.ListSinceCalledWith, []int{7})
		state := decodeBody[map[string]json.RawMessage](t, recorder)
		assert.Contains(t, string(state["chargers"]), validChargerID)
		assert.Contains(t, string(state["trace"]), "something happened")
		assert.Contains(t, string(state["behaviors"]), "fault_mid_session")
		assert.Contains(t, string(state["clock"]), `"speed":1`)
		assert.Equal(t, string(state["world_id"]), `"WORLD-1"`)
	})
}

func TestGetMetrics(t *testing.T) {
	t.Run("serves the counters together with gauges derived from the world", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.metricsGateway.Add(metrics.PushesSent, 12)
		f.chargerController.ListChargersResult = []entity.Charger{
			{ChargerID: "A", State: entity.ChargerStateCharging},
			{ChargerID: "B", State: entity.ChargerStateAvailable},
			{ChargerID: "C", State: entity.ChargerStateAvailable},
		}
		f.sessionController.ListSessionsResult = []entity.Session{
			{SessionID: "SES-1", State: entity.SessionStateActive},
			{SessionID: "SES-2", State: entity.SessionStateCompleted},
		}

		// When
		recorder := serve(t, f, http.MethodGet, "/api/metrics", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		served := decodeBody[map[string]int64](t, recorder)
		assert.Equal(t, served[metrics.PushesSent], int64(12))
		assert.Equal(t, served["chargers"], int64(3))
		assert.Equal(t, served["chargers_available"], int64(2))
		assert.Equal(t, served["chargers_charging"], int64(1))
		assert.Equal(t, served["sessions_active"], int64(1))
		assert.Equal(t, served["sessions_kept"], int64(2))
	})
}
