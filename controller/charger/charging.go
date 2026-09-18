package charger

import (
	"fmt"
	"math"

	"cposim/controller/session"
	"cposim/entity"
)

func (c *controller) StartCharging(input StartChargingInput) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(input.ChargerID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle == nil {
		return entity.Session{}, fmt.Errorf("charger %q has no vehicle plugged in", input.ChargerID)
	}

	if charger.State != entity.ChargerStatePreparing && charger.State != entity.ChargerStateFinishing {
		return entity.Session{}, fmt.Errorf("charger %q is not ready to charge: charger state %q", input.ChargerID, charger.State)
	}

	startedSession, err := c.sessionController.StartSession(session.StartSessionInput{
		AuthorizationReference: input.AuthorizationReference,
		ChargerID:              charger.ChargerID,
		PricePerKWH:            charger.PricePerKWH,
		SiteID:                 charger.SiteID,
		Token:                  input.Token,
	})
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.StartSession: %w", err)
	}

	charger.ConnectorLocked = true
	charger.SessionID = startedSession.SessionID
	charger.State = entity.ChargerStateCharging

	if err := c.updateCharger(&charger); err != nil {
		return entity.Session{}, fmt.Errorf("updateCharger: %w", err)
	}

	return startedSession, nil
}

func (c *controller) StopCharging(sessionID string) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	activeSession, err := c.sessionController.GetSession(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.GetSession: %w", err)
	}

	charger, err := c.chargerRepository.Get(activeSession.ChargerID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != sessionID {
		return entity.Session{}, fmt.Errorf("session %q is not active on charger %q", sessionID, charger.ChargerID)
	}

	if err := c.meterSinceLastTick(charger); err != nil {
		return entity.Session{}, fmt.Errorf("meterSinceLastTick: %w", err)
	}

	stoppedSession, err := c.endSession(&charger, entity.StopReasonRemote, entity.ChargerStateFinishing)
	if err != nil {
		return entity.Session{}, fmt.Errorf("endSession: %w", err)
	}

	return stoppedSession, nil
}

// endSession stops the charger's active session and moves the charger to nextState. A faulted
// charger keeps its connector locked: the stuck cable is part of the scenario.
func (c *controller) endSession(
	charger *entity.Charger,
	stopReason entity.StopReason,
	nextState entity.ChargerState,
) (entity.Session, error) {
	stoppedSession, err := c.sessionController.StopSession(charger.SessionID, stopReason)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionController.StopSession: %w", err)
	}

	if charger.Vehicle != nil {
		vehicle := *charger.Vehicle
		vehicle.StateOfCharge = stateOfChargeAfter(vehicle, stoppedSession.EnergyDeliveredKWH)
		charger.Vehicle = &vehicle
	}

	charger.ConnectorLocked = nextState == entity.ChargerStateFaulted
	charger.SessionID = ""
	charger.State = nextState

	if err := c.updateCharger(charger); err != nil {
		return entity.Session{}, fmt.Errorf("updateCharger: %w", err)
	}

	return stoppedSession, nil
}

func stateOfChargeAfter(vehicle entity.Vehicle, energyDeliveredKWH float64) float64 {
	return math.Min(1, vehicle.StateOfCharge+energyDeliveredKWH/vehicle.BatteryCapacityKWH)
}
