package charger

import (
	"fmt"

	"cposim/controller/behavior"
	"cposim/entity"
)

func (c *controller) AddCharger(input AddChargerInput) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.siteRepository.Get(input.SiteID); err != nil {
		return entity.Charger{}, fmt.Errorf("siteRepository.Get: %w", err)
	}

	if _, err := behavior.Build(input.Behaviors); err != nil {
		return entity.Charger{}, fmt.Errorf("behavior.Build: %w", err)
	}

	if input.MaxPowerKW < 0 || input.MaxPowerKW > maxPowerKW {
		return entity.Charger{}, fmt.Errorf("max power must be between 0 and %d kW: power %v kW", maxPowerKW, input.MaxPowerKW)
	}

	if input.PricePerKWH < 0 || input.PricePerKWH > maxPricePerKWH {
		return entity.Charger{}, fmt.Errorf("price must be between 0 and %d per kWh: price %v", maxPricePerKWH, input.PricePerKWH)
	}

	if input.ChargerID != "" && !validID.MatchString(input.ChargerID) {
		return entity.Charger{}, fmt.Errorf("charger ID must be 1-36 letters, digits, '_' or '-': charger ID %q", input.ChargerID)
	}

	chargers, err := c.chargerRepository.List()
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.List: %w", err)
	}

	if len(chargers) >= c.config.MaxChargers {
		return entity.Charger{}, fmt.Errorf("charger limit reached: limit %d", c.config.MaxChargers)
	}

	if input.Behaviors == nil {
		input.Behaviors = append([]entity.BehaviorSpec{}, c.config.DefaultBehaviors...)
	}

	charger := entity.Charger{
		Behaviors:   input.Behaviors,
		ChargerID:   input.ChargerID,
		MaxPowerKW:  input.MaxPowerKW,
		PricePerKWH: input.PricePerKWH,
		SiteID:      input.SiteID,
		State:       entity.ChargerStateAvailable,
	}

	if charger.ChargerID == "" {
		charger.ChargerID = c.identifierGateway.NewID(chargerIDPrefix)
	} else if _, err := c.chargerRepository.Get(charger.ChargerID); err == nil {
		return entity.Charger{}, fmt.Errorf("charger %q already exists", charger.ChargerID)
	}

	if charger.MaxPowerKW == 0 {
		charger.MaxPowerKW = c.config.DefaultMaxPowerKW
	}

	if charger.PricePerKWH == 0 {
		charger.PricePerKWH = c.config.DefaultPricePerKWH
	}

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) updateCharger(charger *entity.Charger) error {
	charger.UpdatedAt = c.clockGateway.Now()

	if err := c.chargerRepository.Upsert(*charger); err != nil {
		return fmt.Errorf("chargerRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishChargerEvent(*charger); err != nil {
		return fmt.Errorf("eventsGateway.PublishChargerEvent: %w", err)
	}

	return nil
}

func (c *controller) RemoveCharger(chargerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if charger.SessionID != "" {
		return fmt.Errorf("charger %q has an active session: session %q", chargerID, charger.SessionID)
	}

	if err := c.chargerRepository.Delete(chargerID); err != nil {
		return fmt.Errorf("chargerRepository.Delete: %w", err)
	}

	charger.UpdatedAt = c.clockGateway.Now()

	if err := c.eventsGateway.PublishChargerRemovedEvent(charger); err != nil {
		return fmt.Errorf("eventsGateway.PublishChargerRemovedEvent: %w", err)
	}

	return nil
}

func (c *controller) UpdateBehaviors(chargerID string, behaviors []entity.BehaviorSpec) (entity.Charger, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	if _, err := behavior.Build(behaviors); err != nil {
		return entity.Charger{}, fmt.Errorf("behavior.Build: %w", err)
	}

	charger.Behaviors = behaviors

	if err := c.updateCharger(&charger); err != nil {
		return entity.Charger{}, fmt.Errorf("updateCharger: %w", err)
	}

	return charger, nil
}

func (c *controller) GetCharger(chargerID string) (entity.Charger, error) {
	charger, err := c.chargerRepository.Get(chargerID)
	if err != nil {
		return entity.Charger{}, fmt.Errorf("chargerRepository.Get: %w", err)
	}

	return charger, nil
}

func (c *controller) ListChargers() ([]entity.Charger, error) {
	chargers, err := c.chargerRepository.List()
	if err != nil {
		return nil, fmt.Errorf("chargerRepository.List: %w", err)
	}

	return chargers, nil
}
