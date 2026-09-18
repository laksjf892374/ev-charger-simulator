package api

import (
	"fmt"
	"math"
	"net/http"
	"strings"

	"cposim/entity"
	"cposim/gateway/metrics"
)

// getMetrics serves the process-wide counters plus gauges derived from the current world, so one
// call answers both "is it healthy?" and "what is in it right now?".
func (h handler) getMetrics(w http.ResponseWriter, r *http.Request) {
	state, err := h.stateView(math.MaxInt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Errorf("stateView: %w", err))
		return
	}

	snapshot := h.metricsGateway.Snapshot()
	snapshot[metrics.CDRsKept] = int64(len(state.CDRs))
	snapshot[metrics.Chargers] = int64(len(state.Chargers))
	snapshot[metrics.SessionsKept] = int64(len(state.Sessions))
	snapshot[metrics.Sites] = int64(len(state.Sites))

	for _, viewed := range state.Chargers {
		snapshot[metrics.ChargersInStatePrefix+strings.ToLower(string(viewed.State))]++
	}

	for _, listedSession := range state.Sessions {
		if listedSession.State == entity.SessionStateActive {
			snapshot[metrics.SessionsActive]++
		}
	}

	for _, listedCommand := range state.Commands {
		if listedCommand.State == entity.CommandStatePending {
			snapshot[metrics.CommandsPending]++
		}
	}

	respond(w, http.StatusOK, snapshot)
}
