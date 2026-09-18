// Package api is the simulator's control API: everything a person (or a CI script) can do to the
// simulated world. The UI has no private endpoints; it is just one client of this API.
package api

import (
	"fmt"
	"net/http"

	"cposim/controller/behavior"
	"cposim/entity"
)

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

func (h handler) listCommands(w http.ResponseWriter, r *http.Request) {
	commands, err := h.commandController.ListCommands()
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("commandController.ListCommands: %w", err))
		return
	}

	respond(w, http.StatusOK, commands)
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
