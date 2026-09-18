package charger_test

import (
	"math"
	"testing"

	"cposim/controller/behavior"
	"cposim/controller/charger"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/random"
	"cposim/internal/assert"
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
		MaxChargers:        3,
		MaxSites:           2,
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
