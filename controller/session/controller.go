package session

import (
	"fmt"
	"math"
	"sync"
	"time"

	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	cdrrepo "cposim/repository/cdr"
	sessionrepo "cposim/repository/session"
)

const (
	cdrIDPrefix     = "CDR"
	sessionIDPrefix = "SES"
)

type Config struct {
	Currency string
	// How many completed sessions (and their CDRs) are kept; older ones are forgotten. Everything is
	// in memory, so history has to be bounded.
	MaxCompletedSessions  int
	SessionUpdateInterval time.Duration
}

type StartSessionInput struct {
	AuthorizationReference string
	ChargerID              string
	PricePerKWH            float64
	SiteID                 string
	Token                  entity.Token
}

type Controller interface {
	GetSession(sessionID string) (entity.Session, error)
	ListCDRs() ([]entity.CDR, error)
	ListSessions() ([]entity.Session, error)
	RecordSessionProgress(sessionID string, energyDeliveredKWH float64, powerKW float64) (entity.Session, error)
	StartSession(input StartSessionInput) (entity.Session, error)
	StopSession(sessionID string, stopReason entity.StopReason) (entity.Session, error)
}

type controller struct {
	cdrRepository              cdrrepo.Repository
	clockGateway               clock.Gateway
	config                     Config
	eventsGateway              events.Gateway
	identifierGateway          identifier.Gateway
	lastPublishedAtBySessionID map[string]time.Time
	sessionRepository          sessionrepo.Repository
	mu                         sync.Mutex
}

func NewController(
	cdrRepository cdrrepo.Repository,
	clockGateway clock.Gateway,
	config Config,
	eventsGateway events.Gateway,
	identifierGateway identifier.Gateway,
	sessionRepository sessionrepo.Repository,
) (Controller, error) {
	if config.Currency == "" {
		return nil, fmt.Errorf("currency must not be empty")
	}

	if config.MaxCompletedSessions <= 0 {
		return nil, fmt.Errorf("max completed sessions must be positive: max %d", config.MaxCompletedSessions)
	}

	if config.SessionUpdateInterval <= 0 {
		return nil, fmt.Errorf("session update interval must be positive: interval %v", config.SessionUpdateInterval)
	}

	return &controller{
		cdrRepository:              cdrRepository,
		clockGateway:               clockGateway,
		config:                     config,
		eventsGateway:              eventsGateway,
		identifierGateway:          identifierGateway,
		lastPublishedAtBySessionID: map[string]time.Time{},
		sessionRepository:          sessionRepository,
	}, nil
}

func (c *controller) GetSession(sessionID string) (entity.Session, error) {
	session, err := c.sessionRepository.Get(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionRepository.Get: %w", err)
	}

	return session, nil
}

func (c *controller) ListCDRs() ([]entity.CDR, error) {
	cdrs, err := c.cdrRepository.List()
	if err != nil {
		return nil, fmt.Errorf("cdrRepository.List: %w", err)
	}

	return cdrs, nil
}

func (c *controller) ListSessions() ([]entity.Session, error) {
	sessions, err := c.sessionRepository.List()
	if err != nil {
		return nil, fmt.Errorf("sessionRepository.List: %w", err)
	}

	return sessions, nil
}

func (c *controller) RecordSessionProgress(
	sessionID string,
	energyDeliveredKWH float64,
	powerKW float64,
) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, err := c.getActiveSession(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("getActiveSession: %w", err)
	}

	now := c.clockGateway.Now()
	session.EnergyDeliveredKWH += energyDeliveredKWH
	session.PowerKW = powerKW
	session.TotalCost = cost(session)
	session.UpdatedAt = now

	// Progress is recorded every tick, but listeners only hear about it every
	// SessionUpdateInterval: a real CPO does not push a session update several times a second.
	if now.Sub(c.lastPublishedAtBySessionID[sessionID]) < c.config.SessionUpdateInterval {
		if err := c.sessionRepository.Upsert(session); err != nil {
			return entity.Session{}, fmt.Errorf("sessionRepository.Upsert: %w", err)
		}

		return session, nil
	}

	if err := c.updateSession(session); err != nil {
		return entity.Session{}, fmt.Errorf("updateSession: %w", err)
	}

	return session, nil
}

func (c *controller) getActiveSession(sessionID string) (entity.Session, error) {
	session, err := c.sessionRepository.Get(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("sessionRepository.Get: %w", err)
	}

	if session.State != entity.SessionStateActive {
		return entity.Session{}, fmt.Errorf("session %q is not active: session state %q", sessionID, session.State)
	}

	return session, nil
}

func (c *controller) updateSession(session entity.Session) error {
	if err := c.sessionRepository.Upsert(session); err != nil {
		return fmt.Errorf("sessionRepository.Upsert: %w", err)
	}

	c.lastPublishedAtBySessionID[session.SessionID] = session.UpdatedAt

	if err := c.eventsGateway.PublishSessionEvent(session); err != nil {
		return fmt.Errorf("eventsGateway.PublishSessionEvent: %w", err)
	}

	return nil
}

func (c *controller) StartSession(input StartSessionInput) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if input.ChargerID == "" {
		return entity.Session{}, fmt.Errorf("charger ID must not be empty")
	}

	now := c.clockGateway.Now()
	session := entity.Session{
		AuthorizationReference: input.AuthorizationReference,
		ChargerID:              input.ChargerID,
		PricePerKWH:            input.PricePerKWH,
		SessionID:              c.identifierGateway.NewID(sessionIDPrefix),
		SiteID:                 input.SiteID,
		StartedAt:              now,
		State:                  entity.SessionStateActive,
		Token:                  input.Token,
		UpdatedAt:              now,
	}

	if err := c.updateSession(session); err != nil {
		return entity.Session{}, fmt.Errorf("updateSession: %w", err)
	}

	return session, nil
}

func (c *controller) StopSession(sessionID string, stopReason entity.StopReason) (entity.Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, err := c.getActiveSession(sessionID)
	if err != nil {
		return entity.Session{}, fmt.Errorf("getActiveSession: %w", err)
	}

	now := c.clockGateway.Now()
	session.EndedAt = &now
	session.PowerKW = 0
	session.State = entity.SessionStateCompleted
	session.StopReason = stopReason
	session.TotalCost = cost(session)
	session.UpdatedAt = now

	if err := c.updateSession(session); err != nil {
		return entity.Session{}, fmt.Errorf("updateSession: %w", err)
	}

	delete(c.lastPublishedAtBySessionID, sessionID)

	if err := c.createCDR(session); err != nil {
		return entity.Session{}, fmt.Errorf("createCDR: %w", err)
	}

	if err := c.forgetOldestCompleted(); err != nil {
		return entity.Session{}, fmt.Errorf("forgetOldestCompleted: %w", err)
	}

	return session, nil
}

func (c *controller) createCDR(session entity.Session) error {
	cdr := entity.CDR{
		AuthorizationReference: session.AuthorizationReference,
		CDRID:                  c.identifierGateway.NewID(cdrIDPrefix),
		ChargerID:              session.ChargerID,
		CreatedAt:              *session.EndedAt,
		Currency:               c.config.Currency,
		EndedAt:                *session.EndedAt,
		EnergyDeliveredKWH:     session.EnergyDeliveredKWH,
		PricePerKWH:            session.PricePerKWH,
		SessionID:              session.SessionID,
		SiteID:                 session.SiteID,
		StartedAt:              session.StartedAt,
		Token:                  session.Token,
		TotalCost:              session.TotalCost,
	}

	if err := c.cdrRepository.Upsert(cdr); err != nil {
		return fmt.Errorf("cdrRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishCDREvent(cdr); err != nil {
		return fmt.Errorf("eventsGateway.PublishCDREvent: %w", err)
	}

	return nil
}

// cost is the one place a session is priced. Today that is energy times the price the charger
// had when the session started; tariffs with time, session or idle components would replace this
// function (most likely with a pricing package) and nothing else.
func cost(session entity.Session) float64 {
	return roundToCents(session.EnergyDeliveredKWH * session.PricePerKWH)
}

func roundToCents(amount float64) float64 {
	return math.Round(amount*100) / 100
}

// forgetOldestCompleted drops completed sessions beyond the retention limit, oldest first, along
// with their CDRs. IDs are sequential, so list order is age order. Nothing is published: this is
// the simulator forgetting, not something that happened in the simulated world.
func (c *controller) forgetOldestCompleted() error {
	sessions, err := c.sessionRepository.List()
	if err != nil {
		return fmt.Errorf("sessionRepository.List: %w", err)
	}

	completedSessionIDs := []string{}
	for _, session := range sessions {
		if session.State == entity.SessionStateCompleted {
			completedSessionIDs = append(completedSessionIDs, session.SessionID)
		}
	}

	excess := len(completedSessionIDs) - c.config.MaxCompletedSessions
	if excess <= 0 {
		return nil
	}

	forgottenSessionIDs := map[string]bool{}
	for _, sessionID := range completedSessionIDs[:excess] {
		forgottenSessionIDs[sessionID] = true

		if err := c.sessionRepository.Delete(sessionID); err != nil {
			return fmt.Errorf("sessionRepository.Delete: %w", err)
		}
	}

	cdrs, err := c.cdrRepository.List()
	if err != nil {
		return fmt.Errorf("cdrRepository.List: %w", err)
	}

	for _, cdr := range cdrs {
		if !forgottenSessionIDs[cdr.SessionID] {
			continue
		}

		if err := c.cdrRepository.Delete(cdr.CDRID); err != nil {
			return fmt.Errorf("cdrRepository.Delete: %w", err)
		}
	}

	return nil
}
