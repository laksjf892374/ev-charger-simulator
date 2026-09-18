package command_test

import (
	"errors"
	"testing"

	"cposim/assert"
	"cposim/controller/command"
	"cposim/entity"
)

func TestFakeController(t *testing.T) {
	t.Run("returns the configured errors", func(t *testing.T) {
		// Given
		fakeController := command.NewFakeController()
		fakeController.ListCommandsErr = errors.New("boom")
		fakeController.StartSessionErr = errors.New("boom")
		fakeController.StopSessionErr = errors.New("boom")
		fakeController.TickErr = errors.New("boom")
		fakeController.UnlockConnectorErr = errors.New("boom")

		// When
		_, listCommandsErr := fakeController.ListCommands()
		_, startSessionErr := fakeController.StartSession(command.StartSessionInput{})
		_, stopSessionErr := fakeController.StopSession(command.StopSessionInput{})
		tickErr := fakeController.Tick()
		_, unlockConnectorErr := fakeController.UnlockConnector(command.UnlockConnectorInput{})

		// Then
		assert.Error(t, listCommandsErr)
		assert.Error(t, startSessionErr)
		assert.Error(t, stopSessionErr)
		assert.Error(t, tickErr)
		assert.Error(t, unlockConnectorErr)
	})

	t.Run("records calls and returns the configured results", func(t *testing.T) {
		// Given
		fakeController := command.NewFakeController()
		fakeController.ListCommandsResult = []entity.Command{{CommandID: "CMD-0"}}
		fakeController.StartSessionResult = entity.Command{CommandID: "CMD-1"}
		fakeController.StopSessionResult = entity.Command{CommandID: "CMD-2"}
		fakeController.UnlockConnectorResult = entity.Command{CommandID: "CMD-3"}

		// When
		commands, err := fakeController.ListCommands()
		assert.NoError(t, err)
		startCommand, err := fakeController.StartSession(command.StartSessionInput{ChargerID: "CH-1"})
		assert.NoError(t, err)
		stopCommand, err := fakeController.StopSession(command.StopSessionInput{SessionID: "SES-1"})
		assert.NoError(t, err)
		assert.NoError(t, fakeController.Tick())
		unlockCommand, err := fakeController.UnlockConnector(command.UnlockConnectorInput{ChargerID: "CH-2"})
		assert.NoError(t, err)

		// Then
		assert.Equal(t, commands, []entity.Command{{CommandID: "CMD-0"}})
		assert.Equal(t, startCommand.CommandID, "CMD-1")
		assert.Equal(t, fakeController.StartSessionCalledWith, []command.StartSessionInput{{ChargerID: "CH-1"}})
		assert.Equal(t, stopCommand.CommandID, "CMD-2")
		assert.Equal(t, fakeController.StopSessionCalledWith, []command.StopSessionInput{{SessionID: "SES-1"}})
		assert.Equal(t, fakeController.TickCallCount, 1)
		assert.Equal(t, unlockCommand.CommandID, "CMD-3")
		assert.Equal(t, fakeController.UnlockConnectorCalledWith, []command.UnlockConnectorInput{{ChargerID: "CH-2"}})
	})
}
