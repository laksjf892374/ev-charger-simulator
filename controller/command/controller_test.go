package command_test

import (
	"errors"
	"testing"
	"time"

	"cposim/assert"
	"cposim/behavior"
	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/random"
	commandrepo "cposim/repository/command"
)

const (
	validCallbackReference = "https://emsp.example/commands/START_SESSION/1"
	validChargerID         = "CH-001"
	validSessionID         = "SES-000001"
)

var (
	validConfig = command.Config{
		CommandLatency:      2 * time.Second,
		MaxFinishedCommands: 2,
		StartTimeout:        60 * time.Second,
	}
	validPluggedInCharger = entity.Charger{
		ChargerID: validChargerID,
		State:     entity.ChargerStatePreparing,
		Vehicle:   &entity.Vehicle{BatteryCapacityKWH: 60, MaxPowerKW: 150, StateOfCharge: 0.2},
	}
	validStartSessionInput = command.StartSessionInput{
		AuthorizationReference: "AUTH-1",
		CallbackReference:      validCallbackReference,
		ChargerID:              validChargerID,
		Token:                  entity.Token{UID: "TOKEN-1"},
	}
)

type fixture struct {
	chargerController *charger.FakeController
	clockGateway      *clock.FakeGateway
	commandController command.Controller
	eventsGateway     *events.FakeGateway
	randomGateway     *random.FakeGateway
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	chargerController := charger.NewFakeController()
	chargerController.GetChargerResult = validPluggedInCharger
	chargerController.StartChargingResult = entity.Session{SessionID: validSessionID}
	clockGateway := clock.NewFakeGateway()
	eventsGateway := events.NewFakeGateway()
	randomGateway := random.NewFakeGateway()
	commandController, err := command.NewController(
		chargerController,
		clockGateway,
		commandrepo.NewInMemoryRepository(),
		validConfig,
		eventsGateway,
		identifier.NewSequentialGateway(),
		randomGateway,
	)
	assert.NoError(t, err)

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		eventsGateway:     eventsGateway,
		randomGateway:     randomGateway,
	}
}

func lastCommandEvent(t *testing.T, f fixture) entity.Command {
	t.Helper()

	commandEventCount := len(f.eventsGateway.CommandEvents)
	assert.NotEqual(t, commandEventCount, 0)

	return f.eventsGateway.CommandEvents[commandEventCount-1]
}

func TestNewController(t *testing.T) {
	t.Run("returns an error when the start timeout is not positive", func(t *testing.T) {
		// Given
		config := validConfig
		config.StartTimeout = 0

		// When
		_, err := command.NewController(nil, nil, nil, config, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})
}

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

func TestRetention(t *testing.T) {
	t.Run("forgets the oldest finished commands, but never a pending one", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Vehicle = nil
		pendingCommand, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// When
		f.chargerController.GetChargerResult.Behaviors = []entity.BehaviorSpec{{Kind: behavior.KindRejectStart}}
		for i := 0; i < 3; i++ {
			_, err := f.commandController.StartSession(validStartSessionInput)
			assert.NoError(t, err)
		}

		// Then
		commands, err := f.commandController.ListCommands()
		assert.NoError(t, err)
		commandIDs := []string{}
		for _, listedCommand := range commands {
			commandIDs = append(commandIDs, listedCommand.CommandID)
		}
		assert.Equal(t, commandIDs, []string{pendingCommand.CommandID, "CMD-000003", "CMD-000004"})
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

func TestTick(t *testing.T) {
	t.Run("does not resolve a command before the command latency has passed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.clockGateway.Advance(time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).State, entity.CommandStatePending)
	})

	t.Run("resolves a start as EVSE_INOPERATIVE when the charger is faulted", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.chargerController.GetChargerResult.State = entity.ChargerStateFaulted
		f.clockGateway.Advance(2 * time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultEVSEInoperative)
	})

	t.Run("resolves a start as EVSE_OCCUPIED when the charger already has a session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.chargerController.GetChargerResult.SessionID = "SES-OTHER"
		f.clockGateway.Advance(2 * time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultEVSEOccupied)
	})

	t.Run("resolves a start as FAILED when the charger refuses to start charging", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.chargerController.StartChargingErr = errors.New("boom")
		f.clockGateway.Advance(2 * time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultFailed)
		assert.Equal(t, lastCommandEvent(t, f).Message, "boom")
	})

	t.Run("resolves a start with the behavior's forced result after the behavior's delay", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Behaviors = []entity.BehaviorSpec{{Kind: behavior.KindStartFails}}
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// When
		f.clockGateway.Advance(9 * time.Second)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).State, entity.CommandStatePending)

		// When
		f.clockGateway.Advance(time.Second)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultFailed)
		startChargingCallCount := len(f.chargerController.StartChargingCalledWith)
		assert.Equal(t, startChargingCallCount, 0)
	})

	t.Run("keeps a start waiting for plug-in, then resolves it as TIMEOUT at the deadline", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Vehicle = nil
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// When
		f.clockGateway.Advance(59 * time.Second)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).State, entity.CommandStatePending)

		// When
		f.clockGateway.Advance(time.Second)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultTimeout)
	})

	t.Run("starts charging once the driver plugs in while the start is waiting", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.chargerController.GetChargerResult.Vehicle = nil
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.clockGateway.Advance(10 * time.Second)
		assert.NoError(t, f.commandController.Tick())

		// When
		f.chargerController.GetChargerResult = validPluggedInCharger
		f.clockGateway.Advance(time.Second)
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultAccepted)
	})

	t.Run("resolves a start as ACCEPTED with the new session and publishes the result once", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.clockGateway.Advance(2 * time.Second)

		// When
		assert.NoError(t, f.commandController.Tick())
		assert.NoError(t, f.commandController.Tick())

		// Then
		assert.Equal(t, f.chargerController.StartChargingCalledWith, []charger.StartChargingInput{{
			AuthorizationReference: "AUTH-1",
			ChargerID:              validChargerID,
			Token:                  entity.Token{UID: "TOKEN-1"},
		}})
		resolvedCommand := lastCommandEvent(t, f)
		assert.Equal(t, resolvedCommand.State, entity.CommandStateResolved)
		assert.Equal(t, resolvedCommand.Result, entity.CommandResultAccepted)
		assert.Equal(t, resolvedCommand.SessionID, validSessionID)
		assert.Equal(t, resolvedCommand.CallbackReference, validCallbackReference)
		commandEventCount := len(f.eventsGateway.CommandEvents)
		assert.Equal(t, commandEventCount, 2)
	})

	t.Run("resolves a stop by stopping the session on the charger", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.StopSession(command.StopSessionInput{SessionID: validSessionID})
		assert.NoError(t, err)
		f.clockGateway.Advance(2 * time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, f.chargerController.StopChargingCalledWith, []string{validSessionID})
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultAccepted)
	})

	t.Run("resolves an unlock as FAILED when the charger will not unlock", func(t *testing.T) {
		// Given
		f := newFixture(t)
		_, err := f.commandController.UnlockConnector(command.UnlockConnectorInput{ChargerID: validChargerID})
		assert.NoError(t, err)
		f.chargerController.UnlockConnectorErr = errors.New("boom")
		f.clockGateway.Advance(2 * time.Second)

		// When
		err = f.commandController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, lastCommandEvent(t, f).Result, entity.CommandResultFailed)
	})
}
