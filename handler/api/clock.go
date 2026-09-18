package api

import (
	"fmt"
	"net/http"
	"time"
)

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
