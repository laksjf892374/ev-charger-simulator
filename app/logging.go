package app

import (
	"net/http"

	"cposim/app/mockemsp"
	"cposim/gateway/metrics"
)

// Pages left open poll these every second. Logging each successful poll would bury everything
// else, so they are only logged when they fail.
var pollingPaths = map[string]bool{
	"/api/state":                     true,
	HealthPath:                       true,
	mockemsp.BasePath + "/api/state": true,
}

// logged writes one structured line per request and counts it.
func (s *simulator) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := s.wallNow()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(recorder, r)

		s.metricsGateway.Add(metrics.HTTPRequests, 1)
		if recorder.statusCode >= http.StatusInternalServerError {
			s.metricsGateway.Add(metrics.HTTPServerErrors, 1)
		}

		if pollingPaths[r.URL.Path] && r.Method == http.MethodGet && recorder.statusCode < http.StatusBadRequest {
			return
		}

		s.logger.Info(
			"request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"duration_ms", s.wallNow().Sub(startedAt).Milliseconds(),
			"world_id", s.currentWorld.Load().worldID,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
