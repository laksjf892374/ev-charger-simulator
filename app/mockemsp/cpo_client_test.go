package mockemsp_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
	"cposim/ocpi"
)

func TestPullLocations(t *testing.T) {
	t.Run("returns an error when the CPO answers with an OCPI error status", func(t *testing.T) {
		// Given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"status_code": 3000, "status_message": "boom"}`)
		}))
		defer server.Close()

		// When
		_, err := mockemsp.NewHTTPCPOClient(server.URL).PullLocations()

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CPO refused: OCPI status 3000: boom")
	})

	t.Run("follows the Link header until the last page", func(t *testing.T) {
		// Given
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("offset") == "" {
				w.Header().Set("Link", fmt.Sprintf(`<%s/ocpi/cpo/2.2.1/locations?offset=1>; rel="next"`, server.URL))
				io.WriteString(w, `{"status_code": 1000, "data": [{"id": "SITE-1"}]}`)
				return
			}

			io.WriteString(w, `{"status_code": 1000, "data": [{"id": "SITE-2"}]}`)
		}))
		defer server.Close()

		// When
		locations, err := mockemsp.NewHTTPCPOClient(server.URL).PullLocations()

		// Then
		assert.NoError(t, err)
		locationCount := len(locations)
		assert.Equal(t, locationCount, 2)
		assert.Equal(t, locations[1].ID, "SITE-2")
	})
}

func TestSendCommand(t *testing.T) {
	t.Run("reports an OCPI error status as a refusal rather than a failure to reach the CPO", func(t *testing.T) {
		// Given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"status_code": 2003, "status_message": "unknown EVSE"}`)
		}))
		defer server.Close()

		// When
		response, err := mockemsp.NewHTTPCPOClient(server.URL).SendCommand("START_SESSION", ocpi.StartSession{})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, response.Result, mockemsp.CommandResponseRefused)
		assert.Equal(t, response.Message[0].Text, "OCPI status 2003: unknown EVSE")
	})

	t.Run("posts the command as JSON and returns the CPO's synchronous answer", func(t *testing.T) {
		// Given
		var receivedPath, receivedBody string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedPath, receivedBody = r.URL.Path, string(body)
			io.WriteString(w, `{"status_code": 1000, "data": {"result": "ACCEPTED", "timeout": 90}}`)
		}))
		defer server.Close()

		// When
		response, err := mockemsp.NewHTTPCPOClient(server.URL+"/").SendCommand("STOP_SESSION", ocpi.StopSession{
			ResponseURL: "http://emsp.example/callback",
			SessionID:   "SES-1",
		})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, response, ocpi.CommandResponse{Result: "ACCEPTED", Timeout: 90})
		assert.Equal(t, receivedPath, "/ocpi/cpo/2.2.1/commands/STOP_SESSION")
		assert.Equal(t, receivedBody, `{"response_url":"http://emsp.example/callback","session_id":"SES-1"}`)
	})
}
