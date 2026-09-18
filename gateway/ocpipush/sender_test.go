package ocpipush_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cposim/assert"
	"cposim/gateway/clock"
	"cposim/gateway/ocpipush"
	"cposim/gateway/trace"
)

func TestSend(t *testing.T) {
	t.Run("returns an error and sends nothing when the URL is not http(s)", func(t *testing.T) {
		// Given
		traceGateway := trace.NewFakeGateway()
		sender := ocpipush.NewHTTPSender(clock.NewFakeGateway(), traceGateway)

		// When
		err := sender.Send(ocpipush.Push{Method: http.MethodPost, URL: "file:///etc/passwd"})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not an http(s) URL")
	})

	t.Run("returns an error and traces the response when the eMSP answers with an HTTP error", func(t *testing.T) {
		// Given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		}))
		defer server.Close()
		traceGateway := trace.NewFakeGateway()
		sender := ocpipush.NewHTTPSender(clock.NewFakeGateway(), traceGateway)

		// When
		err := sender.Send(ocpipush.Push{Method: http.MethodPost, URL: server.URL})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "eMSP answered HTTP 500")
		assert.Equal(t, traceGateway.Recorded()[0].StatusCode, http.StatusInternalServerError)
	})

	t.Run("sends the body as JSON and traces the exchange with its summary", func(t *testing.T) {
		// Given
		var receivedMethod, receivedBody, receivedContentType string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedMethod, receivedBody, receivedContentType = r.Method, string(body), r.Header.Get("Content-Type")
			io.WriteString(w, `{"status_code":1000}`)
		}))
		defer server.Close()
		clockGateway := clock.NewFakeGateway()
		traceGateway := trace.NewFakeGateway()
		sender := ocpipush.NewHTTPSender(clockGateway, traceGateway)

		// When
		err := sender.Send(ocpipush.Push{
			Body:    map[string]string{"status": "CHARGING"},
			Method:  http.MethodPut,
			Module:  trace.ModuleLocations,
			Summary: "charger is now CHARGING",
			URL:     server.URL + "/locations/US/SIM/SITE-1/EVSE-1",
		})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, receivedMethod, http.MethodPut)
		assert.Equal(t, receivedBody, `{"status":"CHARGING"}`)
		assert.Equal(t, receivedContentType, "application/json")
		assert.Equal(t, traceGateway.Recorded(), []trace.Entry{{
			Direction:    trace.DirectionOutbound,
			Method:       http.MethodPut,
			Module:       trace.ModuleLocations,
			RecordedAt:   clockGateway.Now(),
			RequestBody:  `{"status":"CHARGING"}`,
			ResponseBody: `{"status_code":1000}`,
			StatusCode:   http.StatusOK,
			Summary:      "charger is now CHARGING",
			URL:          server.URL + "/locations/US/SIM/SITE-1/EVSE-1",
		}})
	})
}
