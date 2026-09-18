// Package api is the simulator's control API: everything a person (or a CI script) can do to the
// simulated world. The UI has no private endpoints; it is just one client of this API.
package api

import (
	"fmt"
	"math"
	"net/http"

	"cposim/controller/charger"
	"cposim/entity"
)

// chargerView is a charger as a person standing next to it would see it: including the live
// state of charge, which the core derives rather than stores.
type chargerView struct {
	entity.Charger
	ActiveSession     *entity.Session `json:"active_session,omitempty"`
	LiveStateOfCharge *float64        `json:"live_state_of_charge,omitempty"`
}

func (h handler) listChargers(w http.ResponseWriter, r *http.Request) {
	views, err := h.chargerViews()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("chargerViews: %w", err))
		return
	}

	respond(w, http.StatusOK, views)
}

func (h handler) chargerViews() ([]chargerView, error) {
	chargers, err := h.chargerController.ListChargers()
	if err != nil {
		return nil, fmt.Errorf("chargerController.ListChargers: %w", err)
	}

	views := make([]chargerView, 0, len(chargers))
	for _, listedCharger := range chargers {
		views = append(views, h.chargerView(listedCharger))
	}

	return views, nil
}

func (h handler) chargerView(viewed entity.Charger) chargerView {
	view := chargerView{Charger: viewed}
	if viewed.Vehicle == nil {
		return view
	}

	stateOfCharge := viewed.Vehicle.StateOfCharge
	if viewed.SessionID != "" {
		// best effort: without the session the view is merely less detailed
		if activeSession, err := h.sessionController.GetSession(viewed.SessionID); err == nil {
			view.ActiveSession = &activeSession
			stateOfCharge += activeSession.EnergyDeliveredKWH / viewed.Vehicle.BatteryCapacityKWH
		}
	}

	stateOfCharge = math.Min(1, stateOfCharge)
	view.LiveStateOfCharge = &stateOfCharge

	return view
}

func (h handler) addCharger(w http.ResponseWriter, r *http.Request) {
	var input charger.AddChargerInput
	if !decode(w, r, &input) {
		return
	}

	addedCharger, err := h.chargerController.AddCharger(input)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, fmt.Errorf("chargerController.AddCharger: %w", err))
		return
	}

	respond(w, http.StatusCreated, h.chargerView(addedCharger))
}

func (h handler) removeCharger(w http.ResponseWriter, r *http.Request) {
	existingCharger, ok := h.findCharger(w, r)
	if !ok {
		return
	}

	if err := h.chargerController.RemoveCharger(existingCharger.ChargerID); err != nil {
		respondError(w, http.StatusConflict, fmt.Errorf("chargerController.RemoveCharger: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h handler) findCharger(w http.ResponseWriter, r *http.Request) (entity.Charger, bool) {
	existingCharger, err := h.chargerController.GetCharger(r.PathValue("charger_id"))
	if err != nil {
		respondError(w, http.StatusNotFound, fmt.Errorf("chargerController.GetCharger: %w", err))
		return entity.Charger{}, false
	}

	return existingCharger, true
}

func (h handler) getCharger(w http.ResponseWriter, r *http.Request) {
	existingCharger, ok := h.findCharger(w, r)
	if !ok {
		return
	}

	respond(w, http.StatusOK, h.chargerView(existingCharger))
}

// performAction is what a person standing at the charger can do. A refused action (unplugging a
// locked cable, say) is 409: the request was fine, the charger's state does not allow it.
func (h handler) performAction(w http.ResponseWriter, r *http.Request) {
	existingCharger, ok := h.findCharger(w, r)
	if !ok {
		return
	}

	var vehicle entity.Vehicle
	if !decode(w, r, &vehicle) {
		return
	}

	var updatedCharger entity.Charger
	var err error

	switch action := r.PathValue("action"); action {
	case ActionClearFault:
		updatedCharger, err = h.chargerController.ClearFault(existingCharger.ChargerID)
	case ActionInjectFault:
		updatedCharger, err = h.chargerController.InjectFault(existingCharger.ChargerID)
	case ActionPlugIn:
		updatedCharger, err = h.chargerController.PlugIn(existingCharger.ChargerID, vehicle)
	case ActionPressStop:
		updatedCharger, err = h.chargerController.PressStopButton(existingCharger.ChargerID)
	case ActionUnplug:
		updatedCharger, err = h.chargerController.Unplug(existingCharger.ChargerID)
	default:
		respondError(w, http.StatusNotFound, fmt.Errorf("unknown action %q", action))
		return
	}

	if err != nil {
		respondError(w, http.StatusConflict, err)
		return
	}

	respond(w, http.StatusOK, h.chargerView(updatedCharger))
}

func (h handler) updateBehaviors(w http.ResponseWriter, r *http.Request) {
	existingCharger, ok := h.findCharger(w, r)
	if !ok {
		return
	}

	behaviors := []entity.BehaviorSpec{}
	if !decode(w, r, &behaviors) {
		return
	}

	updatedCharger, err := h.chargerController.UpdateBehaviors(existingCharger.ChargerID, behaviors)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, fmt.Errorf("chargerController.UpdateBehaviors: %w", err))
		return
	}

	respond(w, http.StatusOK, h.chargerView(updatedCharger))
}
