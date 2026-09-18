package session

import (
	"sync"

	"cposim/entity"
)

type RecordSessionProgressCall struct {
	EnergyDeliveredKWH float64
	PowerKW            float64
	SessionID          string
}

type StopSessionCall struct {
	SessionID  string
	StopReason entity.StopReason
}

type FakeController struct {
	GetSessionCalledWith       []string
	GetSessionErr              error
	GetSessionResult           entity.Session
	ListCDRsErr                error
	ListCDRsResult             []entity.CDR
	ListSessionsErr            error
	ListSessionsResult         []entity.Session
	RecordSessionProgressCalls []RecordSessionProgressCall
	RecordSessionProgressErr   error
	StartSessionCalledWith     []StartSessionInput
	StartSessionErr            error
	StartSessionResult         entity.Session
	StopSessionCalls           []StopSessionCall
	StopSessionErr             error
	StopSessionResult          entity.Session
	mu                         sync.Mutex
}

func NewFakeController() *FakeController {
	return &FakeController{}
}

func (c *FakeController) GetSession(sessionID string) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.GetSessionCalledWith = append(c.GetSessionCalledWith, sessionID)

	return c.GetSessionResult, c.GetSessionErr
}

func (c *FakeController) ListCDRs() ([]entity.CDR, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ListCDRsResult, c.ListCDRsErr
}

func (c *FakeController) ListSessions() ([]entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ListSessionsResult, c.ListSessionsErr
}

// RecordSessionProgress accumulates into GetSessionResult so a multi-tick test sees the session
// grow the way the real controller would make it.
func (c *FakeController) RecordSessionProgress(
	sessionID string,
	energyDeliveredKWH float64,
	powerKW float64,
) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.RecordSessionProgressCalls = append(c.RecordSessionProgressCalls, RecordSessionProgressCall{
		EnergyDeliveredKWH: energyDeliveredKWH,
		PowerKW:            powerKW,
		SessionID:          sessionID,
	})

	if c.RecordSessionProgressErr != nil {
		return entity.Session{}, c.RecordSessionProgressErr
	}

	c.GetSessionResult.EnergyDeliveredKWH += energyDeliveredKWH
	c.GetSessionResult.PowerKW = powerKW

	return c.GetSessionResult, nil
}

func (c *FakeController) StartSession(input StartSessionInput) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StartSessionCalledWith = append(c.StartSessionCalledWith, input)

	return c.StartSessionResult, c.StartSessionErr
}

func (c *FakeController) StopSession(sessionID string, stopReason entity.StopReason) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StopSessionCalls = append(c.StopSessionCalls, StopSessionCall{
		SessionID:  sessionID,
		StopReason: stopReason,
	})

	return c.StopSessionResult, c.StopSessionErr
}
