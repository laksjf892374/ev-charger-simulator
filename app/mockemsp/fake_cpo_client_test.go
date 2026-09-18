package mockemsp_test

import (
	"errors"
	"testing"

	"cposim/app/mockemsp"
	"cposim/internal/assert"
	"cposim/ocpi"
)

func TestFakeCPOClient(t *testing.T) {
	t.Run("returns the configured errors", func(t *testing.T) {
		// Given
		fakeCPOClient := mockemsp.NewFakeCPOClient()
		fakeCPOClient.PullLocationsErr = errors.New("boom")
		fakeCPOClient.SendCommandErr = errors.New("boom")

		// When
		_, pullLocationsErr := fakeCPOClient.PullLocations()
		_, sendCommandErr := fakeCPOClient.SendCommand("START_SESSION", nil)

		// Then
		assert.Error(t, pullLocationsErr)
		assert.Error(t, sendCommandErr)
	})

	t.Run("records commands and returns the configured results", func(t *testing.T) {
		// Given
		fakeCPOClient := mockemsp.NewFakeCPOClient()
		fakeCPOClient.PullLocationsResult = []ocpi.Location{{ID: "SITE-1"}}
		fakeCPOClient.SendCommandResult = ocpi.CommandResponse{Result: "ACCEPTED"}

		// When
		locations, err := fakeCPOClient.PullLocations()
		assert.NoError(t, err)
		response, err := fakeCPOClient.SendCommand("STOP_SESSION", ocpi.StopSession{SessionID: "SES-1"})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, locations, []ocpi.Location{{ID: "SITE-1"}})
		assert.Equal(t, response.Result, "ACCEPTED")
		assert.Equal(t, fakeCPOClient.SendCommandCalls, []mockemsp.SendCommandCall{{
			Kind:    "STOP_SESSION",
			Request: ocpi.StopSession{SessionID: "SES-1"},
		}})
	})
}
