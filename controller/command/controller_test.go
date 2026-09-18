package command_test

import (
	"testing"
	"time"

	"cposim/controller/behavior"
	"cposim/controller/charger"
	"cposim/controller/command"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/metrics"
	"cposim/gateway/random"
	"cposim/internal/assert"
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
	metricsGateway    metrics.Gateway
	randomGateway     *random.FakeGateway
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	chargerController := charger.NewFakeController()
	chargerController.GetChargerResult = validPluggedInCharger
	chargerController.StartChargingResult = entity.Session{SessionID: validSessionID}
	clockGateway := clock.NewFakeGateway()
	eventsGateway := events.NewFakeGateway()
	metricsGateway := metrics.NewInMemoryGateway()
	randomGateway := random.NewFakeGateway()
	commandController, err := command.NewController(
		chargerController,
		clockGateway,
		commandrepo.NewInMemoryRepository(),
		validConfig,
		eventsGateway,
		identifier.NewSequentialGateway(),
		metricsGateway,
		randomGateway,
	)
	assert.NoError(t, err)

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandController: commandController,
		eventsGateway:     eventsGateway,
		metricsGateway:    metricsGateway,
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
		_, err := command.NewController(nil, nil, nil, config, nil, nil, nil, nil)

		// Then
		assert.Error(t, err)
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
