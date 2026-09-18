package command

import (
	"fmt"
	"time"

	"cposim/controller/behavior"
	"cposim/entity"
	"cposim/gateway/metrics"
)

func (c *controller) StartSession(input StartSessionInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	targetCharger, err := c.chargerController.GetCharger(input.ChargerID)
	if err != nil {
		return entity.Command{}, fmt.Errorf("chargerController.GetCharger: %w", err)
	}

	attempt := behavior.StartAttempt{
		Charger:     targetCharger,
		ResultDelay: c.config.CommandLatency,
		Roll:        c.randomGateway.Float64(),
	}
	if err := behavior.ApplyStartInterceptors(targetCharger.Behaviors, &attempt); err != nil {
		return entity.Command{}, fmt.Errorf("behavior.ApplyStartInterceptors: %w", err)
	}

	command := c.newCommand(entity.CommandKindStartSession, input.CallbackReference, attempt.ResultDelay)
	command.AuthorizationReference = input.AuthorizationReference
	command.ChargerID = input.ChargerID
	command.ForcedResult = attempt.ForcedResult
	command.Message = attempt.ResultMessage
	command.Token = input.Token

	// a refusal is final: whatever else the behaviors forced no longer applies
	if attempt.Reject {
		command.ForcedResult = ""
		command.Message = attempt.RejectMessage
		command.State = entity.CommandStateRejected
		c.metricsGateway.Add(metrics.CommandsRejected, 1)
	}

	if err := c.updateCommand(command); err != nil {
		return entity.Command{}, fmt.Errorf("updateCommand: %w", err)
	}

	return command, nil
}

func (c *controller) newCommand(
	kind entity.CommandKind,
	callbackReference string,
	resultDelay time.Duration,
) entity.Command {
	now := c.clockGateway.Now()

	return entity.Command{
		CallbackReference: callbackReference,
		CommandID:         c.identifierGateway.NewID(commandIDPrefix),
		CreatedAt:         now,
		DeadlineAt:        now.Add(c.config.StartTimeout),
		Kind:              kind,
		ReadyAt:           now.Add(resultDelay),
		State:             entity.CommandStatePending,
	}
}

func (c *controller) updateCommand(command entity.Command) error {
	command.UpdatedAt = c.clockGateway.Now()

	if err := c.commandRepository.Upsert(command); err != nil {
		return fmt.Errorf("commandRepository.Upsert: %w", err)
	}

	if err := c.eventsGateway.PublishCommandEvent(command); err != nil {
		return fmt.Errorf("eventsGateway.PublishCommandEvent: %w", err)
	}

	if command.State == entity.CommandStatePending {
		return nil
	}

	if err := c.forgetOldestFinished(); err != nil {
		return fmt.Errorf("forgetOldestFinished: %w", err)
	}

	return nil
}

// forgetOldestFinished drops finished commands beyond the retention limit, oldest first. IDs are
// sequential, so list order is age order. Pending commands are never dropped.
func (c *controller) forgetOldestFinished() error {
	commands, err := c.commandRepository.List()
	if err != nil {
		return fmt.Errorf("commandRepository.List: %w", err)
	}

	finishedCommandIDs := []string{}
	for _, command := range commands {
		if command.State != entity.CommandStatePending {
			finishedCommandIDs = append(finishedCommandIDs, command.CommandID)
		}
	}

	excess := len(finishedCommandIDs) - c.config.MaxFinishedCommands
	if excess <= 0 {
		return nil
	}

	for _, commandID := range finishedCommandIDs[:excess] {
		if err := c.commandRepository.Delete(commandID); err != nil {
			return fmt.Errorf("commandRepository.Delete: %w", err)
		}
	}

	return nil
}

func (c *controller) StopSession(input StopSessionInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if input.SessionID == "" {
		return entity.Command{}, fmt.Errorf("session ID must not be empty")
	}

	command := c.newCommand(entity.CommandKindStopSession, input.CallbackReference, c.config.CommandLatency)
	command.SessionID = input.SessionID

	if err := c.updateCommand(command); err != nil {
		return entity.Command{}, fmt.Errorf("updateCommand: %w", err)
	}

	return command, nil
}

func (c *controller) UnlockConnector(input UnlockConnectorInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.chargerController.GetCharger(input.ChargerID); err != nil {
		return entity.Command{}, fmt.Errorf("chargerController.GetCharger: %w", err)
	}

	command := c.newCommand(entity.CommandKindUnlockConnector, input.CallbackReference, c.config.CommandLatency)
	command.ChargerID = input.ChargerID

	if err := c.updateCommand(command); err != nil {
		return entity.Command{}, fmt.Errorf("updateCommand: %w", err)
	}

	return command, nil
}
