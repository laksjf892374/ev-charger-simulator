// Package mockemsp is a deliberately small eMSP: the backend of a driver's charging app. It exists
// so the simulator can be demonstrated without a real eMSP, and it sits at the edge of the
// system: it talks to the CPO only over HTTP, exactly like a real eMSP would, and nothing
// imports it except the DI root and main.
//
// It is naive on purpose. It believes whatever the CPO pushes, in the order it arrives, which
// makes the consequences of a misbehaving CPO easy to see.
package mockemsp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"cposim/ocpi"
)

const (
	BasePath = "/emsp"
	// Where the CPO pushes to: the eMSP's OCPI receiver endpoints.
	ReceiverPath = BasePath + "/ocpi/" + ocpi.Version

	appPath             = BasePath + "/api"
	maxRequestBodyBytes = 256 * 1024
)

type Config struct {
	// The driver this app belongs to.
	DriverToken ocpi.Token
	// This eMSP's own externally reachable base URL, used to build each command's response_url.
	SelfBaseURL string
}

type handler struct {
	config        Config
	cpoClient     CPOClient
	lastCommandID int
	store         *store
	mu            sync.Mutex
}

// Mock is an http.Handler that can also be asked to pull from the CPO, as a real eMSP does when
// it first connects.
type Mock interface {
	http.Handler
	SyncLocations() error
}

type mock struct {
	*handler
	mux *http.ServeMux
}

func NewMock(config Config, cpoClient CPOClient) Mock {
	config.SelfBaseURL = strings.TrimRight(config.SelfBaseURL, "/")

	h := &handler{
		config:    config,
		cpoClient: cpoClient,
		store:     newStore(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("PUT "+ReceiverPath+"/locations/{country_code}/{party_id}/{location_id}", h.putLocation)
	mux.HandleFunc("PUT "+ReceiverPath+"/locations/{country_code}/{party_id}/{location_id}/{evse_uid}", h.putEVSE)
	mux.HandleFunc("PUT "+ReceiverPath+"/sessions/{country_code}/{party_id}/{session_id}", h.putSession)
	mux.HandleFunc("POST "+ReceiverPath+"/cdrs", h.postCDR)
	mux.HandleFunc("POST "+ReceiverPath+"/commands/{command_type}/{uid}", h.postCommandResult)

	mux.HandleFunc("GET "+appPath+"/state", h.getState)
	mux.HandleFunc("POST "+appPath+"/start", h.postStart)
	mux.HandleFunc("POST "+appPath+"/stop", h.postStop)
	mux.HandleFunc("POST "+appPath+"/sync", h.postSync)
	mux.HandleFunc("POST "+appPath+"/unlock", h.postUnlock)

	return mock{handler: h, mux: mux}
}

func (m mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mux.ServeHTTP(w, r)
}

func (h *handler) SyncLocations() error {
	locations, err := h.cpoClient.PullLocations()
	if err != nil {
		return fmt.Errorf("cpoClient.PullLocations: %w", err)
	}

	for _, location := range locations {
		h.store.putLocation(location)
	}

	return nil
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	body := http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(body).Decode(into); err != nil {
		respondJSON(w, http.StatusBadRequest, ocpi.Response{
			StatusCode:    ocpi.StatusCodeInvalidParams,
			StatusMessage: fmt.Sprintf("request body is not valid JSON: %v", err),
		})
		return false
	}

	return true
}

func respondOCPI(w http.ResponseWriter) {
	respondJSON(w, http.StatusOK, ocpi.Response{StatusCode: ocpi.StatusCodeSuccess})
}

func respondJSON(w http.ResponseWriter, httpStatus int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	// an encode failure here means the client went away; there is nobody left to tell
	_ = json.NewEncoder(w).Encode(body)
}
