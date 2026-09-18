package events_test

import (
	"errors"
	"testing"

	"cposim/assert"
	"cposim/entity"
	"cposim/gateway/events"
)

func TestFakeGateway(t *testing.T) {
	t.Run("returns the configured errors", func(t *testing.T) {
		// Given
		fakeGateway := events.NewFakeGateway()
		fakeGateway.PublishCDREventErr = errors.New("boom")
		fakeGateway.PublishChargerEventErr = errors.New("boom")
		fakeGateway.PublishChargerRemovedErr = errors.New("boom")
		fakeGateway.PublishCommandEventErr = errors.New("boom")
		fakeGateway.PublishSessionEventErr = errors.New("boom")
		fakeGateway.PublishSiteEventErr = errors.New("boom")

		// When / Then
		assert.Error(t, fakeGateway.PublishCDREvent(entity.CDR{}))
		assert.Error(t, fakeGateway.PublishChargerEvent(entity.Charger{}))
		assert.Error(t, fakeGateway.PublishChargerRemovedEvent(entity.Charger{}))
		assert.Error(t, fakeGateway.PublishCommandEvent(entity.Command{}))
		assert.Error(t, fakeGateway.PublishSessionEvent(entity.Session{}))
		assert.Error(t, fakeGateway.PublishSiteEvent(entity.Site{}))
	})

	t.Run("records every published event", func(t *testing.T) {
		// Given
		fakeGateway := events.NewFakeGateway()

		// When
		assert.NoError(t, fakeGateway.PublishCDREvent(entity.CDR{CDRID: "CDR-1"}))
		assert.NoError(t, fakeGateway.PublishChargerEvent(entity.Charger{ChargerID: "CH-1"}))
		assert.NoError(t, fakeGateway.PublishChargerRemovedEvent(entity.Charger{ChargerID: "CH-2"}))
		assert.NoError(t, fakeGateway.PublishCommandEvent(entity.Command{CommandID: "CMD-1"}))
		assert.NoError(t, fakeGateway.PublishSessionEvent(entity.Session{SessionID: "SES-1"}))
		assert.NoError(t, fakeGateway.PublishSiteEvent(entity.Site{SiteID: "SITE-1"}))

		// Then
		assert.Equal(t, fakeGateway.CDREvents, []entity.CDR{{CDRID: "CDR-1"}})
		assert.Equal(t, fakeGateway.ChargerEvents, []entity.Charger{{ChargerID: "CH-1"}})
		assert.Equal(t, fakeGateway.ChargerRemovedEvents, []entity.Charger{{ChargerID: "CH-2"}})
		assert.Equal(t, fakeGateway.CommandEvents, []entity.Command{{CommandID: "CMD-1"}})
		assert.Equal(t, fakeGateway.SessionEvents, []entity.Session{{SessionID: "SES-1"}})
		assert.Equal(t, fakeGateway.SiteEvents, []entity.Site{{SiteID: "SITE-1"}})
	})
}
