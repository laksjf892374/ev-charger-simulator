package charger

import (
	"fmt"
	"math"
	"time"

	"cposim/controller/behavior"
	"cposim/entity"
)

// Tick advances every charging charger by the simulated time elapsed since the previous tick.
// One tick drives all chargers (rather than one scheduled job per session) so a session can end
// itself from inside the tick, and so locks are only ever taken charger → session.
func (c *controller) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clockGateway.Now()
	elapsed := now.Sub(c.lastTickAt)
	c.lastTickAt = now

	chargers, err := c.chargerRepository.List()
	if err != nil {
		return fmt.Errorf("chargerRepository.List: %w", err)
	}

	for _, charger := range chargers {
		if charger.State != entity.ChargerStateCharging {
			continue
		}

		if err := c.advanceCharging(charger, elapsed, now); err != nil {
			return fmt.Errorf("advanceCharging: %w", err)
		}
	}

	return nil
}

func (c *controller) advanceCharging(charger entity.Charger, elapsed time.Duration, now time.Time) error {
	tick, vehicleFull, err := c.meter(charger, elapsed, now)
	if err != nil {
		return fmt.Errorf("meter: %w", err)
	}

	if vehicleFull {
		if _, err := c.endSession(&charger, entity.StopReasonVehicleFull, entity.ChargerStateFinishing); err != nil {
			return fmt.Errorf("endSession: %w", err)
		}

		return nil
	}

	if tick.Fault {
		if err := c.fault(&charger); err != nil {
			return fmt.Errorf("fault: %w", err)
		}
	}

	return nil
}

// meter records the energy the charger delivered over elapsed, and reports what the charger's
// behaviors decided for this moment.
func (c *controller) meter(
	charger entity.Charger,
	elapsed time.Duration,
	now time.Time,
) (behavior.Tick, bool, error) {
	activeSession, err := c.sessionController.GetSession(charger.SessionID)
	if err != nil {
		return behavior.Tick{}, false, fmt.Errorf("sessionController.GetSession: %w", err)
	}

	tick := behavior.Tick{
		Charger:          charger,
		ChargingDuration: now.Sub(activeSession.StartedAt),
		Elapsed:          elapsed,
		PowerFactor:      1,
		Roll:             c.randomGateway.Float64(),
		Session:          activeSession,
	}
	if err := behavior.ApplyTickInterceptors(charger.Behaviors, &tick); err != nil {
		return behavior.Tick{}, false, fmt.Errorf("behavior.ApplyTickInterceptors: %w", err)
	}

	// a session that started part-way through this tick has only been charging since it started
	chargingElapsed := min(elapsed, tick.ChargingDuration)

	vehicle := *charger.Vehicle
	stateOfCharge := StateOfChargeAfter(vehicle, activeSession.EnergyDeliveredKWH)
	powerKW := deliveredPowerKW(charger, vehicle, stateOfCharge) * tick.PowerFactor
	energyKWH := powerKW * chargingElapsed.Hours()

	remainingKWH := (1 - stateOfCharge) * vehicle.BatteryCapacityKWH
	vehicleFull := energyKWH >= remainingKWH
	if vehicleFull {
		energyKWH = remainingKWH
	}

	if _, err := c.sessionController.RecordSessionProgress(charger.SessionID, energyKWH, powerKW); err != nil {
		return behavior.Tick{}, false, fmt.Errorf("sessionController.RecordSessionProgress: %w", err)
	}

	return tick, vehicleFull, nil
}

// meterSinceLastTick accounts for the energy delivered between the last tick and now. Anything
// that ends a session outside a tick calls it first, so the bill does not depend on where in the
// tick interval the session happened to stop.
func (c *controller) meterSinceLastTick(charger entity.Charger) error {
	now := c.clockGateway.Now()

	if _, _, err := c.meter(charger, now.Sub(c.lastTickAt), now); err != nil {
		return fmt.Errorf("meter: %w", err)
	}

	return nil
}

func deliveredPowerKW(charger entity.Charger, vehicle entity.Vehicle, stateOfCharge float64) float64 {
	powerKW := math.Min(charger.MaxPowerKW, vehicle.MaxPowerKW)
	if stateOfCharge <= taperStartStateOfCharge {
		return powerKW
	}

	taperProgress := (stateOfCharge - taperStartStateOfCharge) / (1 - taperStartStateOfCharge)

	return powerKW * (1 - taperProgress*(1-taperFloorPowerFactor))
}
