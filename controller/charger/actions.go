package charger

import (
	"fmt"

	"cposim/entity"
)

func (c *controller) PlugIn(chargerID string, vehicle entity.Vehicle) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle != nil {
		return entity.Charger{}, fmt.Errorf("charger %q already has a vehicle plugged in", chargerID)
	}

	if vehicle == (entity.Vehicle{}) {
		vehicle = c.config.DefaultVehicle
	}

	if err := validateVehicle(vehicle); err != nil {
		return entity.Charger{}, fmt.Errorf("validateVehicle: %w", err)
	}

	charger.Vehicle = &vehicle
	if charger.State == entity.ChargerStateAvailable {
		charger.State = entity.ChargerStatePreparing
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) Unplug(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.Vehicle == nil {
		return entity.Charger{}, fmt.Errorf("charger %q has no vehicle plugged in", chargerID)
	}

	if charger.ConnectorLocked {
		return entity.Charger{}, fmt.Errorf("charger %q has its connector locked: charger state %q", chargerID, charger.State)
	}

	charger.Vehicle = nil
	if charger.State != entity.ChargerStateFaulted {
		charger.State = entity.ChargerStateAvailable
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) PressStopButton(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID == "" {
		return entity.Charger{}, fmt.Errorf("charger %q has no active session: charger state %q", chargerID, charger.State)
	}

	if err := c.meterSinceLastTick(charger); err != nil {
		return entity.Charger{}, fmt.Errorf("meterSinceLastTick: %w", err)
	}

	if _, err := c.endSession(&charger, entity.StopReasonStopButton, entity.ChargerStateFinishing); err != nil {
		return entity.Charger{}, fmt.Errorf("endSession: %w", err)
	}

	return charger, nil
}

func (c *controller) InjectFault(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.State == entity.ChargerStateFaulted {
		return entity.Charger{}, fmt.Errorf("charger %q is already faulted", chargerID)
	}

	if charger.SessionID != "" {
		if err := c.meterSinceLastTick(charger); err != nil {
			return entity.Charger{}, fmt.Errorf("meterSinceLastTick: %w", err)
		}
	}

	if err := c.fault(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("fault: %w", err)
	}

	return charger, nil
}

func (c *controller) fault(charger *entity.Charger) error {
	if charger.SessionID != "" {
		if _, err := c.endSession(charger, entity.StopReasonFault, entity.ChargerStateFaulted); err != nil {
			return fmt.Errorf("endSession: %w", err)
		}

		return nil
	}

	charger.State = entity.ChargerStateFaulted

	if err := c.updateCharger(charger); err != nil {
		return fmt.Errorf("updateCharger: %w", err)
	}

	return nil
}

func (c *controller) ClearFault(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.State != entity.ChargerStateFaulted {
		return entity.Charger{}, fmt.Errorf("charger %q is not faulted: charger state %q", chargerID, charger.State)
	}

	charger.ConnectorLocked = false
	charger.State = entity.ChargerStateAvailable
	if charger.Vehicle != nil {
		charger.State = entity.ChargerStatePreparing
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) UnlockConnector(chargerID string) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != "" {
		return entity.Charger{}, fmt.Errorf("charger %q has an active session: session %q", chargerID, charger.SessionID)
	}

	charger.ConnectorLocked = false

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}
