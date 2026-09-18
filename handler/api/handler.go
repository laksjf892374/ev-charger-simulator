// Package api is the simulator's control API: everything a person (or a CI script) can do to the
// simulated world. The UI has no private endpoints; it is just one client of this API.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cposim/controller/behavior"
	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/metrics"
	"cposim/gateway/trace"
)

const (
	ActionClearFault  = "clear-fault"
	ActionInjectFault = "inject-fault"
	ActionPlugIn      = "plug-in"
	ActionPressStop   = "press-stop"
	ActionUnplug      = "unplug"

	BasePath = "/api"

	maxRequestBodyBytes = 64 * 1024
)

type Config struct {
	// Identifies this world. It changes when the simulator is reset, which tells a client holding
	// state from before (a trace sequence, say) to start over.
	WorldID string
}

type handler struct {
	chargerController charger.Controller
	clockGateway      clock.Gateway
	commandController command.Controller
	config            Config
	metricsGateway    metrics.Gateway
	sessionController session.Controller
	traceGateway      trace.Gateway
}

func NewHandler(
	chargerController charger.Controller,
	clockGateway clock.Gateway,
	commandController command.Controller,
	config Config,
	metricsGateway metrics.Gateway,
	sessionController session.Controller,
	traceGateway trace.Gateway,
) http.Handler {
	h := handler{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		config:            config,
		metricsGateway:    metricsGateway,
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+BasePath+"/behaviors", h.listBehaviors)
	mux.HandleFunc("GET "+BasePath+"/cdrs", h.listCDRs)
	mux.HandleFunc("GET "+BasePath+"/chargers", h.listChargers)
	mux.HandleFunc("POST "+BasePath+"/chargers", h.addCharger)
	mux.HandleFunc("DELETE "+BasePath+"/chargers/{charger_id}", h.removeCharger)
	mux.HandleFunc("GET "+BasePath+"/chargers/{charger_id}", h.getCharger)
	mux.HandleFunc("POST "+BasePath+"/chargers/{charger_id}/actions/{action}", h.performAction)
	mux.HandleFunc("PUT "+BasePath+"/chargers/{charger_id}/behaviors", h.updateBehaviors)
	mux.HandleFunc("GET "+BasePath+"/clock", h.getClock)
	mux.HandleFunc("PUT "+BasePath+"/clock", h.updateClock)
	mux.HandleFunc("GET "+BasePath+"/commands", h.listCommands)
	mux.HandleFunc("GET "+BasePath+"/metrics", h.getMetrics)
	mux.HandleFunc("GET "+BasePath+"/sessions", h.listSessions)
	mux.HandleFunc("GET "+BasePath+"/sites", h.listSites)
	mux.HandleFunc("POST "+BasePath+"/sites", h.addSite)
	mux.HandleFunc("GET "+BasePath+"/state", h.getState)
	mux.HandleFunc("GET "+BasePath+"/trace", h.listTrace)

	return mux
}

type errorResponse struct {
	Error string `json:"error"`
}

func respond(w http.ResponseWriter, httpStatus int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	// an encode failure here means the client went away; there is nobody left to tell
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, httpStatus int, err error) {
	respond(w, httpStatus, errorResponse{Error: err.Error()})
}

// decode accepts an empty body as "all defaults", so `curl -X POST` alone is a valid request.
func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(into); err != nil && !errors.Is(err, io.EOF) {
		respondError(w, http.StatusBadRequest, fmt.Errorf("request body is not valid JSON: %w", err))
		return false
	}

	return true
}

func (h handler) listBehaviors(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, behavior.Catalog())
}

func (h handler) listCDRs(w http.ResponseWriter, r *http.Request) {
	cdrs, err := h.sessionController.ListCDRs()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("sessionController.ListCDRs: %w", err))
		return
	}

	respond(w, http.StatusOK, cdrs)
}

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

type clockView struct {
	Now   time.Time `json:"now"`
	Speed float64   `json:"speed"`
}

func (h handler) getClock(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, h.clockView())
}

func (h handler) clockView() clockView {
	return clockView{
		Now:   h.clockGateway.Now(),
		Speed: h.clockGateway.Speed(),
	}
}

func (h handler) updateClock(w http.ResponseWriter, r *http.Request) {
	var input clockView
	if !decode(w, r, &input) {
		return
	}

	if err := h.clockGateway.SetSpeed(input.Speed); err != nil {
		respondError(w, http.StatusUnprocessableEntity, fmt.Errorf("clockGateway.SetSpeed: %w", err))
		return
	}

	respond(w, http.StatusOK, h.clockView())
}

func (h handler) listCommands(w http.ResponseWriter, r *http.Request) {
	commands, err := h.commandController.ListCommands()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("commandController.ListCommands: %w", err))
		return
	}

	respond(w, http.StatusOK, commands)
}

// getMetrics serves the process-wide counters plus gauges derived from the current world, so one
// call answers both "is it healthy?" and "what is in it right now?".
func (h handler) getMetrics(w http.ResponseWriter, r *http.Request) {
	state, err := h.stateView(math.MaxInt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("stateView: %w", err))
		return
	}

	snapshot := h.metricsGateway.Snapshot()
	snapshot["cdrs_kept"] = int64(len(state.CDRs))
	snapshot["chargers"] = int64(len(state.Chargers))
	snapshot["sessions_kept"] = int64(len(state.Sessions))
	snapshot["sites"] = int64(len(state.Sites))

	for _, viewed := range state.Chargers {
		snapshot["chargers_"+strings.ToLower(string(viewed.State))]++
	}

	for _, listedSession := range state.Sessions {
		if listedSession.State == entity.SessionStateActive {
			snapshot["sessions_active"]++
		}
	}

	for _, listedCommand := range state.Commands {
		if listedCommand.State == entity.CommandStatePending {
			snapshot["commands_pending"]++
		}
	}

	respond(w, http.StatusOK, snapshot)
}

func (h handler) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.sessionController.ListSessions()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("sessionController.ListSessions: %w", err))
		return
	}

	respond(w, http.StatusOK, sessions)
}

func (h handler) listSites(w http.ResponseWriter, r *http.Request) {
	sites, err := h.chargerController.ListSites()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("chargerController.ListSites: %w", err))
		return
	}

	respond(w, http.StatusOK, sites)
}

func (h handler) addSite(w http.ResponseWriter, r *http.Request) {
	var site entity.Site
	if !decode(w, r, &site) {
		return
	}

	addedSite, err := h.chargerController.AddSite(site)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, fmt.Errorf("chargerController.AddSite: %w", err))
		return
	}

	respond(w, http.StatusCreated, addedSite)
}

// stateView is everything a dashboard needs in one poll.
type stateView struct {
	Behaviors []behavior.Info  `json:"behaviors"`
	CDRs      []entity.CDR     `json:"cdrs"`
	Chargers  []chargerView    `json:"chargers"`
	Clock     clockView        `json:"clock"`
	Commands  []entity.Command `json:"commands"`
	Sessions  []entity.Session `json:"sessions"`
	Sites     []entity.Site    `json:"sites"`
	Trace     []trace.Entry    `json:"trace"`
	WorldID   string           `json:"world_id"`
}

func (h handler) getState(w http.ResponseWriter, r *http.Request) {
	sinceSequence, ok := parseSince(w, r)
	if !ok {
		return
	}

	state, err := h.stateView(sinceSequence)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("stateView: %w", err))
		return
	}

	respond(w, http.StatusOK, state)
}

func parseSince(w http.ResponseWriter, r *http.Request) (int, bool) {
	text := r.URL.Query().Get("since")
	if text == "" {
		return 0, true
	}

	sinceSequence, err := strconv.Atoi(text)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Errorf("since is not an integer: %q", text))
		return 0, false
	}

	return sinceSequence, true
}

func (h handler) stateView(sinceSequence int) (stateView, error) {
	cdrs, err := h.sessionController.ListCDRs()
	if err != nil {
		return stateView{}, fmt.Errorf("sessionController.ListCDRs: %w", err)
	}

	chargers, err := h.chargerViews()
	if err != nil {
		return stateView{}, fmt.Errorf("chargerViews: %w", err)
	}

	commands, err := h.commandController.ListCommands()
	if err != nil {
		return stateView{}, fmt.Errorf("commandController.ListCommands: %w", err)
	}

	sessions, err := h.sessionController.ListSessions()
	if err != nil {
		return stateView{}, fmt.Errorf("sessionController.ListSessions: %w", err)
	}

	sites, err := h.chargerController.ListSites()
	if err != nil {
		return stateView{}, fmt.Errorf("chargerController.ListSites: %w", err)
	}

	return stateView{
		Behaviors: behavior.Catalog(),
		CDRs:      cdrs,
		Chargers:  chargers,
		Clock:     h.clockView(),
		Commands:  commands,
		Sessions:  sessions,
		Sites:     sites,
		Trace:     h.traceGateway.ListSince(sinceSequence),
		WorldID:   h.config.WorldID,
	}, nil
}

func (h handler) listTrace(w http.ResponseWriter, r *http.Request) {
	sinceSequence, ok := parseSince(w, r)
	if !ok {
		return
	}

	respond(w, http.StatusOK, h.traceGateway.ListSince(sinceSequence))
}
