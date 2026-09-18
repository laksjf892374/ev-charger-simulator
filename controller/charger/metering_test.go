package charger_test

import (
	"encoding/json"
	"testing"
	"time"

	"cposim/controller/behavior"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/internal/assert"
)

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
