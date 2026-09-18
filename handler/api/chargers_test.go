package api_test

import (
	"errors"
	"net/http"
	"testing"

	"cposim/controller/charger"
	"cposim/entity"
	"cposim/handler/api"
	"cposim/internal/assert"
)

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
