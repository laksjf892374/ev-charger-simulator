package session

import (
	"fmt"

	"cposim/entity"
)

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

	isForgottenBySessionID := map[string]bool{}
	for _, sessionID := range completedSessionIDs[:excess] {
		isForgottenBySessionID[sessionID] = true

		if err := c.sessionRepository.Delete(sessionID); err != nil {
			return fmt.Errorf("sessionRepository.Delete: %w", err)
		}
	}

	cdrs, err := c.cdrRepository.List()
	if err != nil {
		return fmt.Errorf("cdrRepository.List: %w", err)
	}

	for _, cdr := range cdrs {
		if !isForgottenBySessionID[cdr.SessionID] {
			continue
		}

		if err := c.cdrRepository.Delete(cdr.CDRID); err != nil {
			return fmt.Errorf("cdrRepository.Delete: %w", err)
		}
	}

	return nil
}
