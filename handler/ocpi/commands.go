package ocpi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"cposim/controller/command"
	"cposim/entity"
	"cposim/ocpi"
)

const (
	commandResponseNotSupported   = "NOT_SUPPORTED"
	commandResponseUnknownSession = "UNKNOWN_SESSION"
)

func (h handler) postCommand(w http.ResponseWriter, r *http.Request) {
	commandType := entity.CommandKind(r.PathValue("command_type"))
	describe(r, "eMSP sends a %s command", commandType)

	switch commandType {
	case entity.CommandKindStartSession:
		h.postStartSession(w, r)
	case entity.CommandKindStopSession:
		h.postStopSession(w, r)
	case entity.CommandKindUnlockConnector:
		h.postUnlockConnector(w, r)
	default:
		h.respondCommand(w, r, commandResponseNotSupported, fmt.Sprintf("command %q is not supported", commandType))
	}
}

func (h handler) postStartSession(w http.ResponseWriter, r *http.Request) {
	var request ocpi.StartSession
	if !h.decodeCommand(w, r, &request, &request.ResponseURL) {
		return
	}

	describe(r, "eMSP asks the CPO to start charging on %s for driver token %s", request.EVSEUID, request.Token.UID)

	if !h.evseExists(w, request.LocationID, request.EVSEUID) {
		return
	}

	accepted, err := h.commandController.StartSession(command.StartSessionInput{
		AuthorizationReference: request.AuthorizationReference,
		CallbackReference:      request.ResponseURL,
		ChargerID:              request.EVSEUID,
		Token:                  ocpi.EntityToken(request.Token),
	})
	if err != nil {
		h.respondServerError(w, fmt.Errorf("commandController.StartSession: %w", err))
		return
	}

	h.respondAccepted(w, r, accepted)
}

// decodeCommand reads a command body and checks its response_url. On failure it has already
// written the response.
func (h handler) decodeCommand(w http.ResponseWriter, r *http.Request, request any, responseURL *string) bool {
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		h.respond(w, http.StatusBadRequest, ocpi.StatusCodeInvalidParams, fmt.Sprintf("request body is not valid JSON: %v", err), nil)
		return false
	}

	parsedURL, err := url.Parse(*responseURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		h.respond(w, http.StatusBadRequest, ocpi.StatusCodeInvalidParams, fmt.Sprintf("response_url is not an http(s) URL: %q", *responseURL), nil)
		return false
	}

	return true
}

func (h handler) evseExists(w http.ResponseWriter, locationID string, evseUID string) bool {
	evseCharger, err := h.chargerController.GetCharger(evseUID)
	if err != nil || evseCharger.SiteID != locationID {
		h.respond(w, http.StatusNotFound, ocpi.StatusCodeUnknownLocation, fmt.Sprintf("unknown EVSE %q at location %q", evseUID, locationID), nil)
		return false
	}

	return true
}

func (h handler) respondAccepted(w http.ResponseWriter, r *http.Request, accepted entity.Command) {
	if accepted.State == entity.CommandStateRejected {
		h.respondCommand(w, r, ocpi.CommandResponseRejected, accepted.Message)
		return
	}

	h.respondCommand(w, r, ocpi.CommandResponseAccepted, "")
}

func (h handler) respondCommand(w http.ResponseWriter, r *http.Request, result string, message string) {
	response := ocpi.CommandResponse{
		Result:  result,
		Timeout: int(h.config.CommandTimeout.Seconds()),
	}
	if message != "" {
		response.Message = []ocpi.DisplayText{{Language: "en", Text: message}}
	}

	if summary, ok := r.Context().Value(summaryKey{}).(*string); ok {
		outcome := "the real outcome will follow as a separate call"
		if result != ocpi.CommandResponseAccepted {
			outcome = "nothing further will happen"
		}

		*summary += fmt.Sprintf(". CPO answers %s: %s", result, outcome)
	}

	h.respond(w, http.StatusOK, ocpi.StatusCodeSuccess, "", response)
}

func (h handler) postStopSession(w http.ResponseWriter, r *http.Request) {
	var request ocpi.StopSession
	if !h.decodeCommand(w, r, &request, &request.ResponseURL) {
		return
	}

	describe(r, "eMSP asks the CPO to stop session %s", request.SessionID)

	if _, err := h.sessionController.GetSession(request.SessionID); err != nil {
		h.respondCommand(w, r, commandResponseUnknownSession, fmt.Sprintf("unknown session %q", request.SessionID))
		return
	}

	accepted, err := h.commandController.StopSession(command.StopSessionInput{
		CallbackReference: request.ResponseURL,
		SessionID:         request.SessionID,
	})
	if err != nil {
		h.respondServerError(w, fmt.Errorf("commandController.StopSession: %w", err))
		return
	}

	h.respondAccepted(w, r, accepted)
}

func (h handler) postUnlockConnector(w http.ResponseWriter, r *http.Request) {
	var request ocpi.UnlockConnector
	if !h.decodeCommand(w, r, &request, &request.ResponseURL) {
		return
	}

	describe(r, "eMSP asks the CPO to unlock the connector of %s", request.EVSEUID)

	if !h.evseExists(w, request.LocationID, request.EVSEUID) {
		return
	}

	accepted, err := h.commandController.UnlockConnector(command.UnlockConnectorInput{
		CallbackReference: request.ResponseURL,
		ChargerID:         request.EVSEUID,
	})
	if err != nil {
		h.respondServerError(w, fmt.Errorf("commandController.UnlockConnector: %w", err))
		return
	}

	h.respondAccepted(w, r, accepted)
}
