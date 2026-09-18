package api

import (
	"fmt"
	"net/http"
	"strconv"

	"cposim/controller/behavior"
	"cposim/entity"
	"cposim/gateway/trace"
)

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
