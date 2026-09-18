package ocpipush_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"cposim/entity"
	"cposim/gateway/metrics"
	"cposim/gateway/ocpipush"
	"cposim/internal/assert"
	"cposim/ocpi"
	chargerrepo "cposim/repository/charger"
	siterepo "cposim/repository/site"
)

const validEMSPBaseURL = "http://emsp.example/ocpi/2.2.1"

var validConfig = ocpipush.Config{
	EMSPBaseURL: validEMSPBaseURL + "/",
	Mapper:      ocpi.Mapper{CountryCode: "US", Currency: "USD", PartyID: "SIM"},
	QueueSize:   16,
}

type fixture struct {
	metricsGateway metrics.Gateway
	out            *bytes.Buffer
	pushGateway    ocpipush.Gateway
	sender         *ocpipush.FakeSender
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	metricsGateway := metrics.NewInMemoryGateway()
	out := &bytes.Buffer{}
	sender := ocpipush.NewFakeSender()
	siteRepository := siterepo.NewInMemoryRepository()
	assert.NoError(t, siteRepository.Upsert(entity.Site{Name: "Oakland Hub", SiteID: "SITE-1"}))

	pushGateway, err := ocpipush.NewGateway(
		chargerrepo.NewInMemoryRepository(),
		validConfig,
		metricsGateway,
		out,
		sender,
		siteRepository,
	)
	assert.NoError(t, err)
	t.Cleanup(pushGateway.Stop)

	return fixture{
		metricsGateway: metricsGateway,
		out:            out,
		pushGateway:    pushGateway,
		sender:         sender,
	}
}

func TestNewGateway(t *testing.T) {
	t.Run("returns an error when the eMSP base URL is empty", func(t *testing.T) {
		// Given
		config := validConfig
		config.EMSPBaseURL = ""

		// When
		_, err := ocpipush.NewGateway(nil, config, nil, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the queue size is not positive", func(t *testing.T) {
		// Given
		config := validConfig
		config.QueueSize = 0

		// When
		_, err := ocpipush.NewGateway(nil, config, nil, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})
}

func TestPublishChargerEvent(t *testing.T) {
	t.Run("PUTs the EVSE to the eMSP's locations module", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishChargerEvent(entity.Charger{
			ChargerID: "EVSE-1",
			SiteID:    "SITE-1",
			State:     entity.ChargerStateCharging,
		})

		// Then
		assert.NoError(t, err)
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].Method, http.MethodPut)
		assert.Equal(t, pushes[0].URL, validEMSPBaseURL+"/locations/US/SIM/SITE-1/EVSE-1")
		assert.Equal(t, pushes[0].Body.(ocpi.EVSE).Status, "CHARGING")
		assert.Contains(t, pushes[0].Summary, "EVSE-1 is now CHARGING")
	})
}

func TestPublishChargerRemovedEvent(t *testing.T) {
	t.Run("PUTs the EVSE with status REMOVED", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishChargerRemovedEvent(entity.Charger{ChargerID: "EVSE-1", SiteID: "SITE-1"})

		// Then
		assert.NoError(t, err)
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].Body.(ocpi.EVSE).Status, "REMOVED")
	})
}

func TestPublishCommandEvent(t *testing.T) {
	t.Run("pushes nothing for a command that is still pending", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishCommandEvent(entity.Command{
			CallbackReference: "http://emsp.example/callback/1",
			State:             entity.CommandStatePending,
		})
		assert.NoError(t, err)
		assert.NoError(t, f.pushGateway.PublishSiteEvent(entity.Site{SiteID: "SITE-1"}))

		// Then
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].URL, validEMSPBaseURL+"/locations/US/SIM/SITE-1")
	})

	t.Run("POSTs the result of a resolved command to its response URL", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishCommandEvent(entity.Command{
			CallbackReference: "http://emsp.example/callback/1",
			Message:           "no vehicle was plugged in",
			Result:            entity.CommandResultTimeout,
			State:             entity.CommandStateResolved,
		})

		// Then
		assert.NoError(t, err)
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].Method, http.MethodPost)
		assert.Equal(t, pushes[0].URL, "http://emsp.example/callback/1")
		assert.Equal(t, pushes[0].Body.(ocpi.CommandResult).Result, "TIMEOUT")
	})
}

func TestPublishSessionEvent(t *testing.T) {
	t.Run("PUTs the session to the eMSP's sessions module", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishSessionEvent(entity.Session{SessionID: "SES-1", State: entity.SessionStateActive})

		// Then
		assert.NoError(t, err)
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].Method, http.MethodPut)
		assert.Equal(t, pushes[0].URL, validEMSPBaseURL+"/sessions/US/SIM/SES-1")
	})
}

func TestPublishCDREvent(t *testing.T) {
	t.Run("POSTs the CDR, described with the site it happened at", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		err := f.pushGateway.PublishCDREvent(entity.CDR{CDRID: "CDR-1", SiteID: "SITE-1"})

		// Then
		assert.NoError(t, err)
		pushes, err := f.sender.AwaitSends(1)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].Method, http.MethodPost)
		assert.Equal(t, pushes[0].URL, validEMSPBaseURL+"/cdrs")
		assert.Equal(t, pushes[0].Module, "cdrs")
		assert.Equal(t, pushes[0].Body.(ocpi.CDR).CDRLocation.Name, "Oakland Hub")
	})
}

func TestDelivery(t *testing.T) {
	t.Run("keeps delivering in order after a push fails, and reports the failure", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.sender.SendErr = errors.New("boom")

		// When
		assert.NoError(t, f.pushGateway.PublishSessionEvent(entity.Session{SessionID: "SES-1"}))
		assert.NoError(t, f.pushGateway.PublishSessionEvent(entity.Session{SessionID: "SES-2"}))

		// Then
		pushes, err := f.sender.AwaitSends(2)
		assert.NoError(t, err)
		assert.Equal(t, pushes[0].URL, validEMSPBaseURL+"/sessions/US/SIM/SES-1")
		assert.Equal(t, pushes[1].URL, validEMSPBaseURL+"/sessions/US/SIM/SES-2")
		f.pushGateway.Stop()
		assert.Contains(t, f.out.String(), "Error: sender.Send PUT")
		assert.Equal(t, f.metricsGateway.Snapshot()[metrics.PushesFailed], int64(2))
		assert.Equal(t, f.metricsGateway.Snapshot()[metrics.PushesSent], int64(0))
	})
}
