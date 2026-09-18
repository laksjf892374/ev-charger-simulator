package mockemsp

import (
	"fmt"
	"net/http"

	"cposim/ocpi"
)

// The OCPI receiver endpoints: what the CPO pushes to this eMSP.

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
