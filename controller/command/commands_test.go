package command_test

import (
	"errors"
	"testing"
	"time"

	"cposim/controller/behavior"
	"cposim/controller/command"
	"cposim/entity"
	"cposim/internal/assert"
)

func TestStartSession(t *testing.T) {
	t.Run("returns an error when the charger does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerErr = errors.New("boom")

		// When
		startCommand, err := f.commandController.StartSession(validStartSessionInput)

		// Then
		assert.Error(t, err)
		assert.Equal(t, startCommand, entity.Command{})
	})

	t.Run("rejects the command when the charger has the reject_start behavior", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Behaviors = []entity.BehaviorSpec{{Kind: behavior.KindRejectStart}}

		// When
		startCommand, err := f.commandController.StartSession(validStartSessionInput)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, startCommand.State, entity.CommandStateRejected)

		// When
		f.clockGateway.Advance(time.Hour)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		startChargingCallCount := len(f.chargerController.StartChargingCalledWith)
		assert.Equal(t, startChargingCallCount, 0)
	})

	t.Run("rejects the command when one behavior refuses, whatever another behavior forced", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Behaviors = []entity.BehaviorSpec{
			{Kind: behavior.KindRejectStart},
			{Kind: behavior.KindStartFails},
		}

		// When
		startCommand, err := f.commandController.StartSession(validStartSessionInput)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, startCommand.State, entity.CommandStateRejected)
		assert.Equal(t, startCommand.ForcedResult, entity.CommandResult(""))
		assert.Equal(t, startCommand.Message, "charger refused the request")
	})

	t.Run("gives the charger's behaviors one random roll per start attempt", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Behaviors = []entity.BehaviorSpec{{Kind: behavior.KindRealisticReliability}}
		f.randomGateway.Float64Results = []float64{0.01, 0.99}

		// When
		unluckyCommand, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		luckyCommand, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// Then
		assert.Equal(t, unluckyCommand.ForcedResult, entity.CommandResultFailed)
		assert.Equal(t, luckyCommand.ForcedResult, entity.CommandResult(""))
		assert.Equal(t, f.randomGateway.Float64CallCount, 2)
	})

	t.Run("accepts the command as pending without touching the charger yet", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		startCommand, err := f.commandController.StartSession(validStartSessionInput)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, startCommand.CommandID, "CMD-000001")
		assert.Equal(t, startCommand.State, entity.CommandStatePending)
		assert.Equal(t, startCommand.CallbackReference, validCallbackReference)
		startChargingCallCount := len(f.chargerController.StartChargingCalledWith)
		assert.Equal(t, startChargingCallCount, 0)
	})
}

func TestStopSession(t *testing.T) {
	t.Run("returns an error when the session ID is empty", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.commandController.StopSession(command.StopSessionInput{})

		// Then
		assert.Error(t, err)
	})
}

func TestUnlockConnector(t *testing.T) {
	t.Run("returns an error when the charger does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerErr = errors.New("boom")

		// When
		_, err := f.commandController.UnlockConnector(command.UnlockConnectorInput{ChargerID: "missing"})

		// Then
		assert.Error(t, err)
	})
}
