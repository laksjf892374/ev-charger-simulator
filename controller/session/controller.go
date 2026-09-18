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
	Currency              string
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
	session.UpdatedAt = now

	if err := c.updateSession(session); err != nil {
		return entity.Session{}, fmt.Errorf("updateSession: %w", err)
	}

	delete(c.lastPublishedAtBySessionID, sessionID)

	if err := c.createCDR(session); err != nil {
		return entity.Session{}, fmt.Errorf("createCDR: %w", err)
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
		TotalCost:              roundToCents(session.EnergyDeliveredKWH * session.PricePerKWH),
	}

	if err := c.cdrRepository.Upsert(cdr); err != nil {
		return fmt.Errorf("cdrRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishCDREvent(cdr); err != nil {
		return fmt.Errorf("eventsGateway.PublishCDREvent: %w", err)
	}

	return nil
}

func roundToCents(amount float64) float64 {
	return math.Round(amount*100) / 100
}
