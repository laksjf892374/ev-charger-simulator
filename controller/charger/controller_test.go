package charger_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"cposim/assert"
	"cposim/behavior"
	"cposim/controller/charger"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/random"
	chargerrepo "cposim/repository/charger"
	siterepo "cposim/repository/site"
)

const (
	validChargerID = "CH-001"
	validSessionID = "SES-000001"
	validSiteID    = "SITE-001"
)

var (
	validConfig = charger.Config{
		DefaultMaxPowerKW:  50,
		DefaultPricePerKWH: 0.45,
		DefaultVehicle:     validVehicle,
	}
	validVehicle = entity.Vehicle{
		BatteryCapacityKWH: 60,
		MaxPowerKW:         150,
		StateOfCharge:      0.2,
	}
)

type fixture struct {
	chargerController charger.Controller
	clockGateway      *clock.FakeGateway
	eventsGateway     *events.FakeGateway
	randomGateway     *random.FakeGateway
	sessionController *session.FakeController
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	return newFixtureWithConfig(t, validConfig)
}

func newFixtureWithConfig(t *testing.T, config charger.Config) fixture {
	t.Helper()

	clockGateway := clock.NewFakeGateway()
	eventsGateway := events.NewFakeGateway()
	randomGateway := random.NewFakeGateway()
	sessionController := session.NewFakeController()
	chargerController, err := charger.NewController(
		chargerrepo.NewInMemoryRepository(),
		clockGateway,
		config,
		eventsGateway,
		identifier.NewSequentialGateway(),
		randomGateway,
		sessionController,
		siterepo.NewInMemoryRepository(),
	)
	assert.NoError(t, err)

	_, err = chargerController.AddSite(entity.Site{Name: "Test Site", SiteID: validSiteID})
	assert.NoError(t, err)

	return fixture{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		eventsGateway:     eventsGateway,
		randomGateway:     randomGateway,
		sessionController: sessionController,
	}
}

func seedCharger(t *testing.T, f fixture, behaviors ...entity.BehaviorSpec) {
	t.Helper()

	_, err := f.chargerController.AddCharger(charger.AddChargerInput{
		Behaviors: behaviors,
		ChargerID: validChargerID,
		SiteID:    validSiteID,
	})
	assert.NoError(t, err)
}

func seedChargingCharger(t *testing.T, f fixture, vehicle entity.Vehicle, behaviors ...entity.BehaviorSpec) {
	t.Helper()

	seedCharger(t, f, behaviors...)
	_, err := f.chargerController.PlugIn(validChargerID, vehicle)
	assert.NoError(t, err)

	f.sessionController.StartSessionResult = entity.Session{
		ChargerID: validChargerID,
		SessionID: validSessionID,
		StartedAt: f.clockGateway.Now(),
		State:     entity.SessionStateActive,
	}
	f.sessionController.GetSessionResult = f.sessionController.StartSessionResult
	_, err = f.chargerController.StartCharging(charger.StartChargingInput{ChargerID: validChargerID})
	assert.NoError(t, err)
}

func roundToThousandths(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func TestNewController(t *testing.T) {
	t.Run("returns an error when the default vehicle is not valid", func(t *testing.T) {
		// Given
		config := validConfig
		config.DefaultVehicle.BatteryCapacityKWH = 0

		// When
		_, err := charger.NewController(nil, clock.NewFakeGateway(), config, nil, nil, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})
}

func TestDefaultBehaviors(t *testing.T) {
	realistic := []entity.BehaviorSpec{{Kind: behavior.KindRealisticReliability}}

	t.Run("returns an error when a default behavior kind is unknown", func(t *testing.T) {
		// Given
		config := validConfig
		config.DefaultBehaviors = []entity.BehaviorSpec{{Kind: "does_not_exist"}}

		// When
		_, err := charger.NewController(nil, clock.NewFakeGateway(), config, nil, nil, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})

	t.Run("gives the default behaviors to a charger that does not say how to behave", func(t *testing.T) {
		// Given
		config := validConfig
		config.DefaultBehaviors = realistic
		f := newFixtureWithConfig(t, config)

		// When
		addedCharger, err := f.chargerController.AddCharger(charger.AddChargerInput{SiteID: validSiteID})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, addedCharger.Behaviors, realistic)
	})

	t.Run("keeps a charger perfectly reliable when it explicitly asks for no behaviors", func(t *testing.T) {
		// Given
		config := validConfig
		config.DefaultBehaviors = realistic
		f := newFixtureWithConfig(t, config)

		// When
		addedCharger, err := f.chargerController.AddCharger(charger.AddChargerInput{
			Behaviors: []entity.BehaviorSpec{},
			SiteID:    validSiteID,
		})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, addedCharger.Behaviors, []entity.BehaviorSpec{})
	})
}

func TestAddCharger(t *testing.T) {
	t.Run("returns an error when the site does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		addedCharger, err := f.chargerController.AddCharger(charger.AddChargerInput{SiteID: "missing"})

		// Then
		assert.Error(t, err)
		assert.Equal(t, addedCharger, entity.Charger{})
	})

	t.Run("returns an error when a behavior kind is unknown", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.chargerController.AddCharger(charger.AddChargerInput{
			Behaviors: []entity.BehaviorSpec{{Kind: "does_not_exist"}},
			SiteID:    validSiteID,
		})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `unknown behavior kind "does_not_exist"`)
	})

	t.Run("returns an error when the charger ID is already taken", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		_, err := f.chargerController.AddCharger(charger.AddChargerInput{
			ChargerID: validChargerID,
			SiteID:    validSiteID,
		})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `charger "CH-001" already exists`)
	})

	t.Run("fills in defaults, generates an ID and publishes the available charger", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		addedCharger, err := f.chargerController.AddCharger(charger.AddChargerInput{SiteID: validSiteID})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, addedCharger.ChargerID, "EVSE-000001")
		assert.Equal(t, addedCharger.MaxPowerKW, 50.0)
		assert.Equal(t, addedCharger.PricePerKWH, 0.45)
		assert.Equal(t, addedCharger.State, entity.ChargerStateAvailable)
		assert.Equal(t, f.eventsGateway.ChargerEvents, []entity.Charger{addedCharger})
	})
}

func TestRemoveCharger(t *testing.T) {
	t.Run("returns an error when the charger has an active session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		err := f.chargerController.RemoveCharger(validChargerID)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has an active session")
	})

	t.Run("deletes the charger and publishes its removal", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		err := f.chargerController.RemoveCharger(validChargerID)

		// Then
		assert.NoError(t, err)
		_, getErr := f.chargerController.GetCharger(validChargerID)
		assert.Error(t, getErr)
		removedEventCount := len(f.eventsGateway.ChargerRemovedEvents)
		assert.Equal(t, removedEventCount, 1)
	})
}

func TestPlugIn(t *testing.T) {
	t.Run("returns an error when a vehicle is already plugged in", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		_, err := f.chargerController.PlugIn(validChargerID, validVehicle)
		assert.NoError(t, err)

		// When
		_, err = f.chargerController.PlugIn(validChargerID, validVehicle)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the vehicle is not valid", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		vehicle := validVehicle
		vehicle.StateOfCharge = 1

		// When
		_, err := f.chargerController.PlugIn(validChargerID, vehicle)

		// Then
		assert.Error(t, err)
	})

	t.Run("moves an available charger to preparing with the default vehicle", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		pluggedCharger, err := f.chargerController.PlugIn(validChargerID, entity.Vehicle{})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, pluggedCharger.State, entity.ChargerStatePreparing)
		assert.Equal(t, *pluggedCharger.Vehicle, validVehicle)
	})
}

func TestUnplug(t *testing.T) {
	t.Run("returns an error when the connector is locked", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		_, err := f.chargerController.Unplug(validChargerID)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connector locked")
	})

	t.Run("returns the charger to available", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		_, err := f.chargerController.PlugIn(validChargerID, validVehicle)
		assert.NoError(t, err)

		// When
		unpluggedCharger, err := f.chargerController.Unplug(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, unpluggedCharger.State, entity.ChargerStateAvailable)
		assert.Equal(t, unpluggedCharger.Vehicle == nil, true)
	})
}

func TestStartCharging(t *testing.T) {
	t.Run("returns an error when no vehicle is plugged in", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		startedSession, err := f.chargerController.StartCharging(charger.StartChargingInput{ChargerID: validChargerID})

		// Then
		assert.Error(t, err)
		assert.Equal(t, startedSession, entity.Session{})
		startSessionCallCount := len(f.sessionController.StartSessionCalledWith)
		assert.Equal(t, startSessionCallCount, 0)
	})

	t.Run("returns an error when the charger is already charging", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		_, err := f.chargerController.StartCharging(charger.StartChargingInput{ChargerID: validChargerID})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `charger state "CHARGING"`)
	})

	t.Run("returns an error when the session cannot be started", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		_, err := f.chargerController.PlugIn(validChargerID, validVehicle)
		assert.NoError(t, err)
		f.sessionController.StartSessionErr = errors.New("boom")

		// When
		_, err = f.chargerController.StartCharging(charger.StartChargingInput{ChargerID: validChargerID})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sessionController.StartSession: boom")
		unchangedCharger, getErr := f.chargerController.GetCharger(validChargerID)
		assert.NoError(t, getErr)
		assert.Equal(t, unchangedCharger.State, entity.ChargerStatePreparing)
	})

	t.Run("starts a session at the charger's price and locks the connector", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		_, err := f.chargerController.PlugIn(validChargerID, validVehicle)
		assert.NoError(t, err)
		f.sessionController.StartSessionResult = entity.Session{SessionID: validSessionID}

		// When
		startedSession, err := f.chargerController.StartCharging(charger.StartChargingInput{
			AuthorizationReference: "AUTH-1",
			ChargerID:              validChargerID,
			Token:                  entity.Token{UID: "TOKEN-1"},
		})

		// Then
		assert.NoError(t, err)
		assert.Equal(t, startedSession.SessionID, validSessionID)
		assert.Equal(t, f.sessionController.StartSessionCalledWith, []session.StartSessionInput{{
			AuthorizationReference: "AUTH-1",
			ChargerID:              validChargerID,
			PricePerKWH:            0.45,
			SiteID:                 validSiteID,
			Token:                  entity.Token{UID: "TOKEN-1"},
		}})
		chargingCharger, getErr := f.chargerController.GetCharger(validChargerID)
		assert.NoError(t, getErr)
		assert.Equal(t, chargingCharger.State, entity.ChargerStateCharging)
		assert.Equal(t, chargingCharger.ConnectorLocked, true)
		assert.Equal(t, chargingCharger.SessionID, validSessionID)
	})
}

func TestStopCharging(t *testing.T) {
	t.Run("returns an error when the session is not the charger's active session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)
		f.sessionController.GetSessionResult = entity.Session{ChargerID: validChargerID, SessionID: "SES-OLD"}

		// When
		_, err := f.chargerController.StopCharging("SES-OLD")

		// Then
		assert.Error(t, err)
		stopSessionCallCount := len(f.sessionController.StopSessionCalls)
		assert.Equal(t, stopSessionCallCount, 0)
	})

	t.Run("stops the session, unlocks the connector and keeps the charged energy in the vehicle", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)
		f.sessionController.StopSessionResult = entity.Session{EnergyDeliveredKWH: 30, SessionID: validSessionID}

		// When
		stoppedSession, err := f.chargerController.StopCharging(validSessionID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, stoppedSession.SessionID, validSessionID)
		assert.Equal(t, f.sessionController.StopSessionCalls, []session.StopSessionCall{{
			SessionID:  validSessionID,
			StopReason: entity.StopReasonRemote,
		}})
		finishingCharger, getErr := f.chargerController.GetCharger(validChargerID)
		assert.NoError(t, getErr)
		assert.Equal(t, finishingCharger.State, entity.ChargerStateFinishing)
		assert.Equal(t, finishingCharger.ConnectorLocked, false)
		assert.Equal(t, finishingCharger.SessionID, "")
		assert.Equal(t, roundToThousandths(finishingCharger.Vehicle.StateOfCharge), 0.7)
	})
}

func TestPressStopButton(t *testing.T) {
	t.Run("returns an error when the charger has no active session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		_, err := f.chargerController.PressStopButton(validChargerID)

		// Then
		assert.Error(t, err)
	})

	t.Run("meters the energy delivered since the last tick before stopping", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)
		f.clockGateway.Advance(6 * time.Minute)
		assert.NoError(t, f.chargerController.Tick())
		f.clockGateway.Advance(36 * time.Second)

		// When
		_, err := f.chargerController.PressStopButton(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, f.sessionController.RecordSessionProgressCalls[1].EnergyDeliveredKWH, 0.5)
	})

	t.Run("stops the session with the stop button reason", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		stoppedCharger, err := f.chargerController.PressStopButton(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, stoppedCharger.State, entity.ChargerStateFinishing)
		assert.Equal(t, f.sessionController.StopSessionCalls[0].StopReason, entity.StopReasonStopButton)
	})
}

func TestInjectFault(t *testing.T) {
	t.Run("returns an error when the charger is already faulted", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		_, err := f.chargerController.InjectFault(validChargerID)
		assert.NoError(t, err)

		// When
		_, err = f.chargerController.InjectFault(validChargerID)

		// Then
		assert.Error(t, err)
	})

	t.Run("ends the active session and leaves the cable locked until the connector is unlocked", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		faultedCharger, err := f.chargerController.InjectFault(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, faultedCharger.State, entity.ChargerStateFaulted)
		assert.Equal(t, faultedCharger.ConnectorLocked, true)
		assert.Equal(t, f.sessionController.StopSessionCalls[0].StopReason, entity.StopReasonFault)
		_, unplugErr := f.chargerController.Unplug(validChargerID)
		assert.Error(t, unplugErr)

		// When
		_, err = f.chargerController.UnlockConnector(validChargerID)
		assert.NoError(t, err)
		unpluggedCharger, err := f.chargerController.Unplug(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, unpluggedCharger.State, entity.ChargerStateFaulted)
	})
}

func TestClearFault(t *testing.T) {
	t.Run("returns an error when the charger is not faulted", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		_, err := f.chargerController.ClearFault(validChargerID)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns a faulted charger with a vehicle to preparing and unlocks it", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)
		_, err := f.chargerController.InjectFault(validChargerID)
		assert.NoError(t, err)

		// When
		clearedCharger, err := f.chargerController.ClearFault(validChargerID)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, clearedCharger.State, entity.ChargerStatePreparing)
		assert.Equal(t, clearedCharger.ConnectorLocked, false)
	})
}

func TestUnlockConnector(t *testing.T) {
	t.Run("returns an error while a session is active", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)

		// When
		_, err := f.chargerController.UnlockConnector(validChargerID)

		// Then
		assert.Error(t, err)
	})
}

func TestUpdateBehaviors(t *testing.T) {
	t.Run("returns an error when a behavior kind is unknown", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)

		// When
		_, err := f.chargerController.UpdateBehaviors(validChargerID, []entity.BehaviorSpec{{Kind: "does_not_exist"}})

		// Then
		assert.Error(t, err)
	})

	t.Run("replaces the charger's behaviors", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		behaviors := []entity.BehaviorSpec{{Kind: behavior.KindRejectStart}}

		// When
		updatedCharger, err := f.chargerController.UpdateBehaviors(validChargerID, behaviors)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, updatedCharger.Behaviors, behaviors)
	})
}

func TestTick(t *testing.T) {
	t.Run("does nothing for chargers that are not charging", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedCharger(t, f)
		f.clockGateway.Advance(time.Minute)

		// When
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		recordSessionProgressCallCount := len(f.sessionController.RecordSessionProgressCalls)
		assert.Equal(t, recordSessionProgressCallCount, 0)
	})

	t.Run("delivers the charger's max power for the simulated time since the last tick", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle)
		f.clockGateway.Advance(6 * time.Minute)

		// When
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, f.sessionController.RecordSessionProgressCalls, []session.RecordSessionProgressCall{{
			EnergyDeliveredKWH: 5,
			PowerKW:            50,
			SessionID:          validSessionID,
		}})
	})

	t.Run("meters a session only from its start when it began after the previous tick", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.clockGateway.Advance(time.Hour)
		seedChargingCharger(t, f, validVehicle)
		f.clockGateway.Advance(6 * time.Minute)

		// When
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, f.sessionController.RecordSessionProgressCalls[0].EnergyDeliveredKWH, 5.0)
	})

	t.Run("tapers power once the vehicle is above 80% state of charge", func(t *testing.T) {
		// Given
		f := newFixture(t)
		vehicle := validVehicle
		vehicle.StateOfCharge = 0.9
		seedChargingCharger(t, f, vehicle)
		f.clockGateway.Advance(time.Second)

		// When
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		powerKW := roundToThousandths(f.sessionController.RecordSessionProgressCalls[0].PowerKW)
		assert.Equal(t, powerKW, 27.5)
	})

	t.Run("ends the session when the vehicle is full, delivering only the remaining energy", func(t *testing.T) {
		// Given
		f := newFixture(t)
		vehicle := validVehicle
		vehicle.StateOfCharge = 0.99
		seedChargingCharger(t, f, vehicle)
		f.clockGateway.Advance(time.Hour)

		// When
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		energyDeliveredKWH := roundToThousandths(f.sessionController.RecordSessionProgressCalls[0].EnergyDeliveredKWH)
		assert.Equal(t, energyDeliveredKWH, 0.6)
		assert.Equal(t, f.sessionController.StopSessionCalls, []session.StopSessionCall{{
			SessionID:  validSessionID,
			StopReason: entity.StopReasonVehicleFull,
		}})
		finishedCharger, getErr := f.chargerController.GetCharger(validChargerID)
		assert.NoError(t, getErr)
		assert.Equal(t, finishedCharger.State, entity.ChargerStateFinishing)
	})

	t.Run("faults a realistically reliable charger only when its roll comes up", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle, entity.BehaviorSpec{
			Kind:   behavior.KindRealisticReliability,
			Params: json.RawMessage(`{"session_faults_per_hour": 1}`),
		})
		// a 6-minute tick at 1 fault per hour is a 10% chance
		f.randomGateway.Float64Results = []float64{0.5, 0.05}

		// When
		f.clockGateway.Advance(6 * time.Minute)
		assert.NoError(t, f.chargerController.Tick())

		// Then
		stopSessionCallCount := len(f.sessionController.StopSessionCalls)
		assert.Equal(t, stopSessionCallCount, 0)

		// When
		f.clockGateway.Advance(6 * time.Minute)
		assert.NoError(t, f.chargerController.Tick())

		// Then
		assert.Equal(t, f.sessionController.StopSessionCalls[0].StopReason, entity.StopReasonFault)
	})

	t.Run("faults the charger mid-session when the fault_mid_session behavior is due", func(t *testing.T) {
		// Given
		f := newFixture(t)
		seedChargingCharger(t, f, validVehicle, entity.BehaviorSpec{
			Kind:   behavior.KindFaultMidSession,
			Params: json.RawMessage(`{"after_s": 60}`),
		})

		// When
		f.clockGateway.Advance(30 * time.Second)
		err := f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		stopSessionCallCount := len(f.sessionController.StopSessionCalls)
		assert.Equal(t, stopSessionCallCount, 0)

		// When
		f.clockGateway.Advance(30 * time.Second)
		err = f.chargerController.Tick()

		// Then
		assert.NoError(t, err)
		assert.Equal(t, f.sessionController.StopSessionCalls, []session.StopSessionCall{{
			SessionID:  validSessionID,
			StopReason: entity.StopReasonFault,
		}})
		faultedCharger, getErr := f.chargerController.GetCharger(validChargerID)
		assert.NoError(t, getErr)
		assert.Equal(t, faultedCharger.State, entity.ChargerStateFaulted)
		assert.Equal(t, faultedCharger.ConnectorLocked, true)
	})
}
