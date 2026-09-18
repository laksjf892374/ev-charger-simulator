package charger_test

import (
	"testing"

	"cposim/controller/behavior"
	"cposim/controller/charger"
	"cposim/entity"
	"cposim/internal/assert"
)

func TestAddChargerGuardrails(t *testing.T) {
	t.Run("returns an error when the charger ID would not survive a URL path", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.chargerController.AddCharger(charger.AddChargerInput{ChargerID: "a/b", SiteID: validSiteID})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "charger ID must be 1-36 letters")
	})

	t.Run("returns an error when the power or price is implausible", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, powerErr := f.chargerController.AddCharger(charger.AddChargerInput{MaxPowerKW: 1e9, SiteID: validSiteID})
		_, priceErr := f.chargerController.AddCharger(charger.AddChargerInput{PricePerKWH: -1, SiteID: validSiteID})

		// Then
		assert.Error(t, powerErr)
		assert.Error(t, priceErr)
	})

	t.Run("returns an error when the charger limit is reached, and frees a slot on removal", func(t *testing.T) {
		// Given
		f := newFixture(t)
		for i := 0; i < 3; i++ {
			_, err := f.chargerController.AddCharger(charger.AddChargerInput{SiteID: validSiteID})
			assert.NoError(t, err)
		}

		// When
		_, err := f.chargerController.AddCharger(charger.AddChargerInput{SiteID: validSiteID})

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "charger limit reached: limit 3")

		// When
		assert.NoError(t, f.chargerController.RemoveCharger("EVSE-000001"))
		_, err = f.chargerController.AddCharger(charger.AddChargerInput{SiteID: validSiteID})

		// Then
		assert.NoError(t, err)
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
