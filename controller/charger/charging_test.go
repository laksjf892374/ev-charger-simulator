package charger_test

import (
	"errors"
	"testing"

	"cposim/controller/charger"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/internal/assert"
)

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
