package app_test

import (
	"net/http"
	"strings"
	"testing"

	"cposim/app"
	"cposim/internal/assert"
)

func TestFakeEMSP(t *testing.T) {
	t.Run("returns an error when no matching request arrives in time", func(t *testing.T) {
		// Given
		fakeEMSP := app.NewFakeEMSP()
		defer fakeEMSP.Close()

		// When
		_, err := fakeEMSP.AwaitRequest("/cdrs", "")

		// Then
		assert.Error(t, err)
	})

	t.Run("records requests and finds them by path suffix and body", func(t *testing.T) {
		// Given
		fakeEMSP := app.NewFakeEMSP()
		defer fakeEMSP.Close()

		// When
		response, err := http.Post(fakeEMSP.URL+"/ocpi/cdrs", "application/json", strings.NewReader(`{"id": "CDR-1"}`))
		assert.NoError(t, err)
		response.Body.Close()
		request, err := fakeEMSP.AwaitRequest("/cdrs", "CDR-1")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, request, app.FakeEMSPRequest{
			Body:   `{"id": "CDR-1"}`,
			Method: http.MethodPost,
			Path:   "/ocpi/cdrs",
		})
	})
}
