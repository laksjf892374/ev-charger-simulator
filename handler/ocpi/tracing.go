package ocpi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"cposim/gateway/trace"
)

type summaryKey struct{}

// traced records every inbound OCPI exchange. Handlers add the plain-English summary through
// describe; the middleware owns everything that can be captured generically.
func (h handler) traced(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes))
		r.Body = io.NopCloser(bytes.NewReader(requestBody))

		summary := ""
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), summaryKey{}, &summary)))

		if summary == "" {
			summary = "eMSP called an endpoint the CPO does not have"
		}

		h.traceGateway.Record(trace.Entry{
			Direction:    trace.DirectionInbound,
			Method:       r.Method,
			RecordedAt:   h.clockGateway.Now(),
			RequestBody:  string(requestBody),
			ResponseBody: recorder.body.String(),
			StatusCode:   recorder.statusCode,
			Summary:      summary,
			URL:          r.URL.RequestURI(),
		})
	})
}

func describe(r *http.Request, format string, args ...any) {
	if summary, ok := r.Context().Value(summaryKey{}).(*string); ok {
		*summary = fmt.Sprintf(format, args...)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	body       bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body.Write(data)

	return r.ResponseWriter.Write(data)
}
