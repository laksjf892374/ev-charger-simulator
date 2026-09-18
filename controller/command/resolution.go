package command

import (
	"fmt"
	"time"

	"cposim/controller/charger"
	"cposim/entity"
	"cposim/gateway/metrics"
)

// Tick resolves every pending command that is due.
func (c *controller) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	commands, err := c.commandRepository.List()
	if err != nil {
		return fmt.Errorf("commandRepository.List: %w", err)
	}

	now := c.clockGateway.Now()
	for _, command := range commands {
		if command.State != entity.CommandStatePending || now.Before(command.ReadyAt) {
			continue
		}

		if err := c.resolveIfDue(command, now); err != nil {
			return fmt.Errorf("resolveIfDue: %w", err)
		}
	}

	return nil
}

func (c *controller) resolveIfDue(command entity.Command, now time.Time) error {
	result, message := c.attempt(&command, now)
	if result == "" {
		return nil
	}

	command.Message = message
	command.Result = result
	command.State = entity.CommandStateResolved
	c.metricsGateway.Add(metrics.CommandsResolvedPrefix+string(result), 1)

	if err := c.updateCommand(command); err != nil {
		return fmt.Errorf("updateCommand: %w", err)
	}

	return nil
}

// attempt carries the command out against the charger. An empty result means "not yet": a
// remote start keeps waiting for the driver to plug in until its deadline.
func (c *controller) attempt(command *entity.Command, now time.Time) (entity.CommandResult, string) {
	if command.ForcedResult != "" {
		return command.ForcedResult, command.Message
	}

	switch command.Kind {
	case entity.CommandKindStartSession:
		return c.attemptStart(command, now)
	case entity.CommandKindStopSession:
		if _, err := c.chargerController.StopCharging(command.SessionID); err != nil {
			return entity.CommandResultFailed, err.Error()
		}

		return entity.CommandResultAccepted, ""
	case entity.CommandKindUnlockConnector:
		if _, err := c.chargerController.UnlockConnector(command.ChargerID); err != nil {
			return entity.CommandResultFailed, err.Error()
		}

		return entity.CommandResultAccepted, ""
	}

	return entity.CommandResultFailed, fmt.Sprintf("unsupported command kind %q", command.Kind)
}

func (c *controller) attemptStart(command *entity.Command, now time.Time) (entity.CommandResult, string) {
	targetCharger, err := c.chargerController.GetCharger(command.ChargerID)
	if err != nil {
		return entity.CommandResultFailed, err.Error()
	}

	if targetCharger.State == entity.ChargerStateFaulted {
		return entity.CommandResultEVSEInoperative, "charger is faulted"
	}

	if targetCharger.SessionID != "" {
		return entity.CommandResultEVSEOccupied, "charger already has an active session"
	}

	if targetCharger.Vehicle == nil {
		if now.Before(command.DeadlineAt) {
			return "", ""
		}

		return entity.CommandResultTimeout, "no vehicle was plugged in before the start timeout"
	}

	startedSession, err := c.chargerController.StartCharging(charger.StartChargingInput{
		AuthorizationReference: command.AuthorizationReference,
		ChargerID:              command.ChargerID,
		Token:                  command.Token,
	})
	if err != nil {
		return entity.CommandResultFailed, err.Error()
	}

	command.SessionID = startedSession.SessionID

	return entity.CommandResultAccepted, ""
}
