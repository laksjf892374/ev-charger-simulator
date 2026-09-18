// Package api is the simulator's control API: everything a person (or a CI script) can do to the
// simulated world. The UI has no private endpoints; it is just one client of this API.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
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
