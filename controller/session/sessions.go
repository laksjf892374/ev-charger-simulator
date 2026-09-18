package session

import (
	"fmt"

	"cposim/entity"
)

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
