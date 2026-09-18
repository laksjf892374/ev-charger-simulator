package session_test

import (
	"errors"
	"testing"
	"time"

	"cposim/assert"
	"cposim/controller/session"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	cdrrepo "cposim/repository/cdr"
	sessionrepo "cposim/repository/session"
)

const (
	validChargerID   = "CH-001"
	validPricePerKWH = 0.5
	validSiteID      = "SITE-001"
)

var (
	validConfig = session.Config{
		Currency:              "EUR",
		MaxCompletedSessions:  2,
		SessionUpdateInterval: 30 * time.Second,
	}
	validStartSessionInput = session.StartSessionInput{
		AuthorizationReference: "AUTH-1",
		ChargerID:              validChargerID,
		PricePerKWH:            validPricePerKWH,
		SiteID:                 validSiteID,
		Token:                  entity.Token{UID: "TOKEN-1"},
	}
)

type fixture struct {
	clockGateway      *clock.FakeGateway
	eventsGateway     *events.FakeGateway
	sessionController session.Controller
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	clockGateway := clock.NewFakeGateway()
	eventsGateway := events.NewFakeGateway()
	sessionController, err := session.NewController(
		cdrrepo.NewInMemoryRepository(),
		clockGateway,
		validConfig,
		eventsGateway,
		identifier.NewSequentialGateway(),
		sessionrepo.NewInMemoryRepository(),
	)
	assert.NoError(t, err)

	return fixture{
		clockGateway:      clockGateway,
		eventsGateway:     eventsGateway,
		sessionController: sessionController,
	}
}

func TestNewController(t *testing.T) {
	t.Run("returns an error when the currency is empty", func(t *testing.T) {
		// Given
		config := validConfig
		config.Currency = ""

		// When
		_, err := session.NewController(nil, nil, config, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the retention limit is not positive", func(t *testing.T) {
		// Given
		config := validConfig
		config.MaxCompletedSessions = 0

		// When
		_, err := session.NewController(nil, nil, config, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the session update interval is not positive", func(t *testing.T) {
		// Given
		config := validConfig
		config.SessionUpdateInterval = 0

		// When
		_, err := session.NewController(nil, nil, config, nil, nil, nil)

		// Then
		assert.Error(t, err)
	})
}

func TestRetention(t *testing.T) {
	t.Run("forgets the oldest completed sessions and their CDRs, but never an active session", func(t *testing.T) {
		// Given
		f := newFixture(t)
		activeSession, err := f.sessionController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// When
		for i := 0; i < 3; i++ {
			completedSession, err := f.sessionController.StartSession(validStartSessionInput)
			assert.NoError(t, err)
			_, err = f.sessionController.StopSession(completedSession.SessionID, entity.StopReasonRemote)
			assert.NoError(t, err)
		}

		// Then
		sessions, err := f.sessionController.ListSessions()
		assert.NoError(t, err)
		sessionIDs := []string{}
		for _, listedSession := range sessions {
			sessionIDs = append(sessionIDs, listedSession.SessionID)
		}
		assert.Equal(t, sessionIDs, []string{activeSession.SessionID, "SES-000003", "SES-000004"})

		cdrs, err := f.sessionController.ListCDRs()
		assert.NoError(t, err)
		cdrSessionIDs := []string{}
		for _, cdr := range cdrs {
			cdrSessionIDs = append(cdrSessionIDs, cdr.SessionID)
		}
		assert.Equal(t, cdrSessionIDs, []string{"SES-000003", "SES-000004"})
	})
}

func TestStartSession(t *testing.T) {
	t.Run("returns an error when the charger ID is empty", func(t *testing.T) {
		// Given
		f := newFixture(t)
		input := validStartSessionInput
		input.ChargerID = ""

		// When
		startedSession, err := f.sessionController.StartSession(input)

		// Then
		assert.Error(t, err)
		assert.Equal(t, startedSession, entity.Session{})
	})

	t.Run("returns an error when the session event cannot be published", func(t *testing.T) {
		// Given
		f := newFixture(t)
		f.eventsGateway.PublishSessionEventErr = errors.New("boom")

		// When
		_, err := f.sessionController.StartSession(validStartSessionInput)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "updateSession: eventsGateway.PublishSessionEvent: boom")
	})

	t.Run("creates an active session and publishes it", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		startedSession, err := f.sessionController.StartSession(validStartSessionInput)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, startedSession.SessionID, "SES-000001")
		assert.Equal(t, startedSession.State, entity.SessionStateActive)
		assert.Equal(t, startedSession.StartedAt, f.clockGateway.Now())
		assert.Equal(t, f.eventsGateway.SessionEvents, []entity.Session{startedSession})
	})
}

func TestRecordSessionProgress(t *testing.T) {
	t.Run("returns an error when the session does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		_, err := f.sessionController.RecordSessionProgress("missing", 1, 50)

		// Then
		assert.Error(t, err)
	})

	t.Run("returns an error when the session is already completed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		startedSession, err := f.sessionController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		_, err = f.sessionController.StopSession(startedSession.SessionID, entity.StopReasonRemote)
		assert.NoError(t, err)

		// When
		_, err = f.sessionController.RecordSessionProgress(startedSession.SessionID, 1, 50)

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `is not active: session state "COMPLETED"`)
	})

	t.Run("accumulates energy every time but publishes only once per update interval", func(t *testing.T) {
		// Given
		f := newFixture(t)
		startedSession, err := f.sessionController.StartSession(validStartSessionInput)
		assert.NoError(t, err)

		// When
		f.clockGateway.Advance(10 * time.Second)
		_, err = f.sessionController.RecordSessionProgress(startedSession.SessionID, 0.25, 50)

		// Then
		assert.NoError(t, err)
		sessionEventCount := len(f.eventsGateway.SessionEvents)
		assert.Equal(t, sessionEventCount, 1)

		// When
		f.clockGateway.Advance(20 * time.Second)
		progressedSession, err := f.sessionController.RecordSessionProgress(startedSession.SessionID, 0.5, 48)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, progressedSession.EnergyDeliveredKWH, 0.75)
		assert.Equal(t, progressedSession.PowerKW, 48.0)
		assert.Equal(t, progressedSession.TotalCost, 0.38)
		sessionEventCount = len(f.eventsGateway.SessionEvents)
		assert.Equal(t, sessionEventCount, 2)
		assert.Equal(t, f.eventsGateway.SessionEvents[1], progressedSession)
	})
}

func TestStopSession(t *testing.T) {
	t.Run("returns an error when the session does not exist", func(t *testing.T) {
		// Given
		f := newFixture(t)

		// When
		stoppedSession, err := f.sessionController.StopSession("missing", entity.StopReasonRemote)

		// Then
		assert.Error(t, err)
		assert.Equal(t, stoppedSession, entity.Session{})
	})

	t.Run("returns an error when the session is already completed", func(t *testing.T) {
		// Given
		f := newFixture(t)
		startedSession, err := f.sessionController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		_, err = f.sessionController.StopSession(startedSession.SessionID, entity.StopReasonRemote)
		assert.NoError(t, err)

		// When
		_, err = f.sessionController.StopSession(startedSession.SessionID, entity.StopReasonRemote)

		// Then
		assert.Error(t, err)
		cdrEventCount := len(f.eventsGateway.CDREvents)
		assert.Equal(t, cdrEventCount, 1)
	})

	t.Run("completes the session and issues a CDR priced from the delivered energy", func(t *testing.T) {
		// Given
		f := newFixture(t)
		startedSession, err := f.sessionController.StartSession(validStartSessionInput)
		assert.NoError(t, err)
		f.clockGateway.Advance(30 * time.Minute)
		_, err = f.sessionController.RecordSessionProgress(startedSession.SessionID, 24.333, 50)
		assert.NoError(t, err)

		// When
		stoppedSession, err := f.sessionController.StopSession(startedSession.SessionID, entity.StopReasonStopButton)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, stoppedSession.State, entity.SessionStateCompleted)
		assert.Equal(t, stoppedSession.StopReason, entity.StopReasonStopButton)
		assert.Equal(t, stoppedSession.PowerKW, 0.0)
		assert.Equal(t, *stoppedSession.EndedAt, f.clockGateway.Now())

		cdrs, err := f.sessionController.ListCDRs()
		assert.NoError(t, err)
		assert.Equal(t, cdrs, f.eventsGateway.CDREvents)
		cdrCount := len(cdrs)
		assert.Equal(t, cdrCount, 1)
		assert.Equal(t, cdrs[0].SessionID, startedSession.SessionID)
		assert.Equal(t, cdrs[0].EnergyDeliveredKWH, 24.333)
		assert.Equal(t, cdrs[0].TotalCost, 12.17)
		assert.Equal(t, stoppedSession.TotalCost, cdrs[0].TotalCost)
		assert.Equal(t, cdrs[0].Currency, "EUR")
	})
}
