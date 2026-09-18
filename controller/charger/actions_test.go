package charger_test

import (
	"testing"
	"time"

	"cposim/entity"
	"cposim/internal/assert"
)

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
