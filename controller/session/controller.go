package session

import (
	"fmt"
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
