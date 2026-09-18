// Package ocpi is the inbound half of the OCPI adapter: the CPO-side HTTP endpoints an eMSP calls.
package ocpi

import (
	"encoding/json"
	"net/http"
	"time"

	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/controller/session"
	"cposim/gateway/clock"
	"cposim/gateway/trace"
	"cposim/ocpi"
)

const (
	BasePath   = "/ocpi"
	modulePath = BasePath + "/cpo/" + ocpi.Version

	maxRequestBodyBytes = 64 * 1024
)

type Config struct {
	// Advertised to the eMSP as how long it should wait for a command result.
	CommandTimeout time.Duration
	Mapper         ocpi.Mapper
}

type handler struct {
	chargerController charger.Controller
	clockGateway      clock.Gateway
	commandController command.Controller
	config            Config
	sessionController session.Controller
	traceGateway      trace.Gateway
}

func NewHandler(
	chargerController charger.Controller,
	clockGateway clock.Gateway,
	commandController command.Controller,
	config Config,
	sessionController session.Controller,
	traceGateway trace.Gateway,
) http.Handler {
	h := handler{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		config:            config,
		sessionController: sessionController,
		traceGateway:      traceGateway,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+BasePath+"/versions", h.getVersions)
	mux.HandleFunc("GET "+BasePath+"/"+ocpi.Version, h.getVersionDetails)
	mux.HandleFunc("GET "+modulePath+"/locations", h.listLocations)
	mux.HandleFunc("GET "+modulePath+"/locations/{location_id}", h.getLocation)
	mux.HandleFunc("GET "+modulePath+"/locations/{location_id}/{evse_uid}", h.getEVSE)
	mux.HandleFunc("GET "+modulePath+"/sessions", h.listSessions)
	mux.HandleFunc("GET "+modulePath+"/cdrs", h.listCDRs)
	mux.HandleFunc("POST "+modulePath+"/commands/{command_type}", h.postCommand)

	return h.traced(authenticated(mux))
}

// authenticated is where OCPI's "Authorization: Token …" check belongs. Credentials are out of
// scope for now, so every request is let through.
func authenticated(next http.Handler) http.Handler {
	return next
}

func (h handler) respond(w http.ResponseWriter, httpStatus int, ocpiStatusCode int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	// an encode failure here means the client went away; there is nobody left to tell
	_ = json.NewEncoder(w).Encode(ocpi.Response{
		Data:          data,
		StatusCode:    ocpiStatusCode,
		StatusMessage: message,
		Timestamp:     ocpi.Timestamp(h.clockGateway.Now()),
	})
}

func (h handler) respondServerError(w http.ResponseWriter, err error) {
	h.respond(w, http.StatusInternalServerError, ocpi.StatusCodeServerError, err.Error(), nil)
}
