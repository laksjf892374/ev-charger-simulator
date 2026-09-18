package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cposim/assert"
	"cposim/handler/web"
)

func TestNewHandler(t *testing.T) {
	t.Run("serves the embedded page at the root", func(t *testing.T) {
		// Given
		handler, err := web.NewHandler()
		assert.NoError(t, err)
		recorder := httptest.NewRecorder()

		// When
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

		// Then
		assert.Equal(t, recorder.Code, http.StatusOK)
		assert.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, recorder.Body.String(), "<title>EV Charging Simulator</title>")
	})

	t.Run("answers not found for a file that is not embedded", func(t *testing.T) {
		// Given
		handler, err := web.NewHandler()
		assert.NoError(t, err)
		recorder := httptest.NewRecorder()

		// When
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing.js", nil))

		// Then
		assert.Equal(t, recorder.Code, http.StatusNotFound)
	})
}
