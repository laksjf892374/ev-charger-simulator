package session_test

import (
	"errors"
	"testing"

	"cposim/controller/session"
	"cposim/entity"
	"cposim/internal/assert"
)

func TestFakeController(t *testing.T) {
	t.Run("returns the configured errors", func(t *testing.T) {
		// Given
		fakeController := session.NewFakeController()
		fakeController.GetSessionErr = errors.New("boom")
		fakeController.ListCDRsErr = errors.New("boom")
		fakeController.ListSessionsErr = errors.New("boom")
		fakeController.RecordSessionProgressErr = errors.New("boom")
		fakeController.StartSessionErr = errors.New("boom")
		fakeController.StopSessionErr = errors.New("boom")

		// When
		_, getSessionErr := fakeController.GetSession("SES-1")
		_, listCDRsErr := fakeController.ListCDRs()
		_, listSessionsErr := fakeController.ListSessions()
		_, recordSessionProgressErr := fakeController.RecordSessionProgress("SES-1", 1, 50)
		_, startSessionErr := fakeController.StartSession(session.StartSessionInput{})
		_, stopSessionErr := fakeController.StopSession("SES-1", entity.StopReasonRemote)

		// Then
		assert.Error(t, getSessionErr)
		assert.Error(t, listCDRsErr)
		assert.Error(t, listSessionsErr)
		assert.Error(t, recordSessionProgressErr)
		assert.Error(t, startSessionErr)
		assert.Error(t, stopSessionErr)
	})

	t.Run("records calls and returns the configured results", func(t *testing.T) {
		// Given
		fakeController := session.NewFakeController()
		fakeController.GetSessionResult = entity.Session{SessionID: "SES-1"}
		fakeController.ListCDRsResult = []entity.CDR{{CDRID: "CDR-1"}}
		fakeController.ListSessionsResult = []entity.Session{{SessionID: "SES-1"}}
		fakeController.StartSessionResult = entity.Session{SessionID: "SES-2"}
		fakeController.StopSessionResult = entity.Session{SessionID: "SES-3"}

		// When
		gotSession, err := fakeController.GetSession("SES-1")
		assert.NoError(t, err)
		cdrs, err := fakeController.ListCDRs()
		assert.NoError(t, err)
		sessions, err := fakeController.ListSessions()
		assert.NoError(t, err)
		startedSession, err := fakeController.StartSession(session.StartSessionInput{ChargerID: "CH-1"})
		assert.NoError(t, err)
		stoppedSession, err := fakeController.StopSession("SES-3", entity.StopReasonFault)
		assert.NoError(t, err)

		// Then
		assert.Equal(t, gotSession, entity.Session{SessionID: "SES-1"})
		assert.Equal(t, fakeController.GetSessionCalledWith, []string{"SES-1"})
		assert.Equal(t, cdrs, []entity.CDR{{CDRID: "CDR-1"}})
		assert.Equal(t, sessions, []entity.Session{{SessionID: "SES-1"}})
		assert.Equal(t, startedSession, entity.Session{SessionID: "SES-2"})
		assert.Equal(t, fakeController.StartSessionCalledWith, []session.StartSessionInput{{ChargerID: "CH-1"}})
		assert.Equal(t, stoppedSession, entity.Session{SessionID: "SES-3"})
		assert.Equal(t, fakeController.StopSessionCalls, []session.StopSessionCall{{
			SessionID:  "SES-3",
			StopReason: entity.StopReasonFault,
		}})
	})

	t.Run("accumulates recorded progress into the session it returns", func(t *testing.T) {
		// Given
		fakeController := session.NewFakeController()
		fakeController.GetSessionResult = entity.Session{SessionID: "SES-1"}

		// When
		_, err := fakeController.RecordSessionProgress("SES-1", 1, 50)
		assert.NoError(t, err)
		progressedSession, err := fakeController.RecordSessionProgress("SES-1", 0.5, 40)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, progressedSession.EnergyDeliveredKWH, 1.5)
		assert.Equal(t, progressedSession.PowerKW, 40.0)
		recordSessionProgressCallCount := len(fakeController.RecordSessionProgressCalls)
		assert.Equal(t, recordSessionProgressCallCount, 2)
	})
}
