package command

import (
	"fmt"

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
