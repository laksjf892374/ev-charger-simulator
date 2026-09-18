package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/metrics"
	"cposim/gateway/trace"
	"cposim/handler/api"
	"cposim/internal/assert"
)

const validChargerID = "EVSE-000001"

type fixture struct {
	chargerController *charger.FakeController
	clockGateway      *clock.FakeGateway
	handler           http.Handler
	metricsGateway    metrics.Gateway
	sessionController *session.FakeController
	traceGateway      *trace.FakeGateway
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	chargerController := charger.NewFakeController()
	chargerController.GetChargerResult = entity.Charger{ChargerID: validChargerID}
	clockGateway := clock.NewFakeGateway()
	metricsGateway := metrics.NewInMemoryGateway()
	sessionController := session.NewFakeController()
	traceGateway := trace.NewFakeGateway()

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		handler: api.NewHandler(
			chargerController,
			clockGateway,
			command.NewFakeController(),
			api.Config{WorldID: "WORLD-1"},
			metricsGateway,
			sessionController,
			traceGateway,
		),
		metricsGateway:    metricsGateway,
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}
}

func serve(t *testing.T, f fixture, method string, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))

	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var body T
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))

	return body
}

func TestAddCharger(t *testing.T) {
	t.Run("rejects a body that is not JSON", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers", "not json")

		// Then
		assert.Equal(t, recorder.Code, http.StatusBadRequest)
	})

	t.Run("reports why the controller refused the charger", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.PhysicalActionErr = errors.New("boom")

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers", `{"site_id": "SITE-1"}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusUnprocessableEntity)
		assert.Contains(t, recorder.Body.String(), "chargerController.AddCharger: boom")
	})

	t.Run("accepts an empty body as a charger with all defaults", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusCreated)
		assert.Equal(t, f.chargerController.PhysicalActionCalls, []charger.PhysicalActionCall{{Action: "AddCharger"}})
	})
}

func TestPerformAction(t *testing.T) {
	t.Run("answers not found for a charger that does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerErr = errors.New("boom")

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers/missing/actions/plug-in", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
	})

	t.Run("answers not found for an action that does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers/"+validChargerID+"/actions/kick", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
		assert.Contains(t, recorder.Body.String(), `unknown action \"kick\"`)
	})

	t.Run("answers conflict when the charger's state does not allow the action", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.PhysicalActionErr = errors.New("connector locked")

		// When
		recorder := serve(t, f, http.MethodPost, "/api/chargers/"+validChargerID+"/actions/unplug", "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusConflict)
		assert.Contains(t, recorder.Body.String(), "connector locked")
	})

	t.Run("dispatches every action to the matching controller method", func(t *testing.T) {
		// Given
		f := newFixture(t)
		actions := []string{api.ActionPlugIn, api.ActionPressStop, api.ActionUnplug, api.ActionInjectFault, api.ActionClearFault}

		// When
		for _, action := range actions {
			recorder := serve(t, f, http.MethodPost, "/api/chargers/"+validChargerID+"/actions/"+action, "")
			assert.Equal(t, recorder.Code, http.StatusOK)
		}

		// Then
		assert.Equal(t, f.chargerController.PhysicalActionCalls, []charger.PhysicalActionCall{
			{Action: "PlugIn", ChargerID: validChargerID},
			{Action: "PressStopButton", ChargerID: validChargerID},
			{Action: "Unplug", ChargerID: validChargerID},
			{Action: "InjectFault", ChargerID: validChargerID},
			{Action: "ClearFault", ChargerID: validChargerID},
		})
	})
}

func TestGetCharger(t *testing.T) {
	t.Run("adds the live state of charge derived from the active session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult = entity.Charger{
			ChargerID: validChargerID,
			SessionID: "SES-1",
			Vehicle:   &entity.Vehicle{BatteryCapacityKWH: 50, MaxPowerKW: 100, StateOfCharge: 0.25},
		}
		f.sessionController.GetSessionResult = entity.Session{EnergyDeliveredKWH: 25, SessionID: "SES-1"}

		// When
		recorder := serve(t, f, http.MethodGet, "/api/chargers/"+validChargerID, "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		view := decodeBody[map[string]any](t, recorder)
		assert.Equal(t, view["live_state_of_charge"], 0.75)
		assert.Equal(t, view["charger_id"], validChargerID)
		assert.Equal(t, view["active_session"].(map[string]any)["session_id"], "SES-1")
	})
}

func TestRemoveCharger(t *testing.T) {
	t.Run("answers conflict when the charger cannot be removed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.PhysicalActionErr = errors.New("has an active session")

		// When
		recorder := serve(t, f, http.MethodDelete, "/api/chargers/"+validChargerID, "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusConflict)
	})

	t.Run("answers no content once removed", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodDelete, "/api/chargers/"+validChargerID, "")

		// Then
		assert.Equal(t, recorder.Code, http.StatusNoContent)
	})
}

func TestUpdateBehaviors(t *testing.T) {
	t.Run("reports why the controller refused the behaviors", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.PhysicalActionErr = errors.New("unknown behavior kind")

		// When
		recorder := serve(t, f, http.MethodPut, "/api/chargers/"+validChargerID+"/behaviors", `[{"kind": "nope"}]`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusUnprocessableEntity)
	})
}

func TestUpdateClock(t *testing.T) {
	t.Run("reports why the clock refused the speed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.clockGateway.SetSpeedErr = errors.New("speed must be positive")

		// When
		recorder := serve(t, f, http.MethodPut, "/api/clock", `{"speed": 0}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusUnprocessableEntity)
	})

	t.Run("sets the simulation speed", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		recorder := serve(t, f, http.MethodPut, "/api/clock", `{"speed": 60}`)

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Equal(t, f.clockGateway.SetSpeedCalledWith, []float64{60})
	})
}

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
