package mockemsp

// The app endpoints: what the driver's phone calls.

import (
	"fmt"
	"net/http"

	"cposim/ocpi"
)

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
