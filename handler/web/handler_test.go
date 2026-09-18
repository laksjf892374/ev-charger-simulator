package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cposim/handler/web"
	"cposim/internal/assert"
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
		assert.Contains(t, recorder.Body.String(), `href="app.css"`)
		assert.Contains(t, recorder.Body.String(), `src="app.js"`)
	})

	t.Run("serves the stylesheet and the script the page refers to", func(t *testing.T) {
		// Given
		handler, err := web.NewHandler()
		assert.NoError(t, err)

		for _, file := range []struct{ path, contentType, content string }{
			{"/app.css", "text/css", ".charger{"},
			{"/app.js", "javascript", "function poll()"},
		} {
			recorder := httptest.NewRecorder()

			// When
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, file.path, nil))

			// Then
			assert.Equal(t, recorder.Code, http.StatusOK)
			assert.Contains(t, recorder.Header().Get("Content-Type"), file.contentType)
			assert.Contains(t, recorder.Body.String(), file.content)
		}
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
