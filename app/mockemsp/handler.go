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

// OCPI receiver endpoints: what the CPO pushes.

func (h *handler) putLocation(w http.ResponseWriter, r *http.Request) {
	var location ocpi.Location
	if !decode(w, r, &location) {
		return
	}

	location.ID = r.PathValue("location_id")
	h.store.putLocation(location)

	respondOCPI(w)
}

func (h *handler) putEVSE(w http.ResponseWriter, r *http.Request) {
	var evse ocpi.EVSE
	if !decode(w, r, &evse) {
		return
	}

	evse.UID = r.PathValue("evse_uid")
	h.store.putEVSE(r.PathValue("location_id"), evse)

	respondOCPI(w)
}

func (h *handler) putSession(w http.ResponseWriter, r *http.Request) {
	var session ocpi.Session
	if !decode(w, r, &session) {
		return
	}

	session.ID = r.PathValue("session_id")
	h.store.putSession(session)

	respondOCPI(w)
}

func (h *handler) postCDR(w http.ResponseWriter, r *http.Request) {
	var cdr ocpi.CDR
	if !decode(w, r, &cdr) {
		return
	}

	h.store.addCDR(cdr)

	respondOCPI(w)
}

func (h *handler) postCommandResult(w http.ResponseWriter, r *http.Request) {
	var result ocpi.CommandResult
	if !decode(w, r, &result) {
		return
	}

	if !h.store.resolveCommand(r.PathValue("uid"), result) {
		respondJSON(w, http.StatusNotFound, ocpi.Response{
			StatusCode:    ocpi.StatusCodeClientError,
			StatusMessage: fmt.Sprintf("unknown command %q", r.PathValue("uid")),
		})
		return
	}

	respondOCPI(w)
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

// App endpoints: what the driver's phone calls.

type errorResponse struct {
	Error string `json:"error"`
}

func (h *handler) getState(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.store.state())
}

type evseRequest struct {
	EVSEUID    string `json:"evse_uid"`
	LocationID string `json:"location_id"`
}

func (h *handler) postStart(w http.ResponseWriter, r *http.Request) {
	var request evseRequest
	if !decode(w, r, &request) {
		return
	}

	kind := "START_SESSION"
	uid := h.newCommandUID()
	h.send(w, Command{EVSEUID: request.EVSEUID, Kind: kind, UID: uid}, ocpi.StartSession{
		EVSEUID:     request.EVSEUID,
		LocationID:  request.LocationID,
		ResponseURL: h.responseURL(kind, uid),
		Token:       h.config.DriverToken,
	})
}

func (h *handler) newCommandUID() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastCommandID++

	return fmt.Sprintf("REQ-%04d", h.lastCommandID)
}

func (h *handler) responseURL(kind string, uid string) string {
	return h.config.SelfBaseURL + ReceiverPath + "/commands/" + kind + "/" + uid
}

// send records the command before sending it, so a result that races ahead of the CPO's
// synchronous answer still finds its command.
func (h *handler) send(w http.ResponseWriter, command Command, request any) {
	command.Response = CommandResultPending
	command.Result = CommandResultPending
	h.store.addCommand(command)

	response, err := h.cpoClient.SendCommand(command.Kind, request)
	if err != nil {
		h.store.answerCommand(command.UID, "ERROR", err.Error())
		respondJSON(w, http.StatusBadGateway, errorResponse{Error: fmt.Sprintf("cpoClient.SendCommand: %v", err)})
		return
	}

	message := ""
	if len(response.Message) > 0 {
		message = response.Message[0].Text
	}

	respondJSON(w, http.StatusOK, h.store.answerCommand(command.UID, response.Result, message))
}

func (h *handler) postStop(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SessionID string `json:"session_id"`
	}
	if !decode(w, r, &request) {
		return
	}

	kind := "STOP_SESSION"
	uid := h.newCommandUID()
	h.send(w, Command{Kind: kind, SessionID: request.SessionID, UID: uid}, ocpi.StopSession{
		ResponseURL: h.responseURL(kind, uid),
		SessionID:   request.SessionID,
	})
}

func (h *handler) postUnlock(w http.ResponseWriter, r *http.Request) {
	var request evseRequest
	if !decode(w, r, &request) {
		return
	}

	kind := "UNLOCK_CONNECTOR"
	uid := h.newCommandUID()
	h.send(w, Command{EVSEUID: request.EVSEUID, Kind: kind, UID: uid}, ocpi.UnlockConnector{
		ConnectorID: ocpi.ConnectorID,
		EVSEUID:     request.EVSEUID,
		LocationID:  request.LocationID,
		ResponseURL: h.responseURL(kind, uid),
	})
}

func (h *handler) postSync(w http.ResponseWriter, r *http.Request) {
	if err := h.SyncLocations(); err != nil {
		respondJSON(w, http.StatusBadGateway, errorResponse{Error: fmt.Sprintf("SyncLocations: %v", err)})
		return
	}

	respondJSON(w, http.StatusOK, h.store.state())
}
