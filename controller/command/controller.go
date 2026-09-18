package command

import (
	"fmt"
	"sync"
	"time"

	"cposim/behavior"
	"cposim/controller/charger"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	commandrepo "cposim/repository/command"
)

const commandIDPrefix = "CMD"

type Config struct {
	// Simulated time between accepting a command and the charger acting on it.
	CommandLatency time.Duration
	// How long a remote start waits for the driver to plug in before resolving as TIMEOUT.
	StartTimeout time.Duration
}

type StartSessionInput struct {
	AuthorizationReference string
	CallbackReference      string
	ChargerID              string
	Token                  entity.Token
}

type StopSessionInput struct {
	CallbackReference string
	SessionID         string
}

type UnlockConnectorInput struct {
	CallbackReference string
	ChargerID         string
}

// Controller owns the lifecycle of remote commands: the synchronous accept/reject, and the
// asynchronous result that follows an accepted command. Rejection is a command state, not an
// error; errors mean the request itself could not be understood.
type Controller interface {
	ListCommands() ([]entity.Command, error)
	StartSession(input StartSessionInput) (entity.Command, error)
	StopSession(input StopSessionInput) (entity.Command, error)
	Tick() error
	UnlockConnector(input UnlockConnectorInput) (entity.Command, error)
}

type controller struct {
	chargerController charger.Controller
	clockGateway      clock.Gateway
	commandRepository commandrepo.Repository
	config            Config
	eventsGateway     events.Gateway
	identifierGateway identifier.Gateway
	mu                sync.Mutex
}

func NewController(
	chargerController charger.Controller,
	clockGateway clock.Gateway,
	commandRepository commandrepo.Repository,
	config Config,
	eventsGateway events.Gateway,
	identifierGateway identifier.Gateway,
) (Controller, error) {
	if config.CommandLatency < 0 {
		return nil, fmt.Errorf("command latency must not be negative: latency %v", config.CommandLatency)
	}

	if config.StartTimeout <= 0 {
		return nil, fmt.Errorf("start timeout must be positive: timeout %v", config.StartTimeout)
	}

	return &controller{
		chargerController: chargerController,
		clockGateway:      clockGateway,
		commandRepository: commandRepository,
		config:            config,
		eventsGateway:     eventsGateway,
		identifierGateway: identifierGateway,
	}, nil
}

func (c *controller) ListCommands() ([]entity.Command, error) {
	commands, err := c.commandRepository.List()
	if err != nil {
		return nil, fmt.Errorf("commandRepository.List: %w", err)
	}

	return commands, nil
}

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
	}
	if err := behavior.ApplyStartInterceptors(targetCharger.Behaviors, &attempt); err != nil {
		return entity.Command{}, fmt.Errorf("behavior.ApplyStartInterceptors: %w", err)
	}

	command := c.newCommand(entity.CommandKindStartSession, input.CallbackReference, attempt.ResultDelay)
	command.AuthorizationReference = input.AuthorizationReference
	command.ChargerID = input.ChargerID
	command.ForcedResult = attempt.ForcedResult
	command.Token = input.Token

	if attempt.Reject {
		command.Message = attempt.RejectMessage
		command.State = entity.CommandStateRejected
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

	if err := c.updateCommand(command); err != nil {
		return fmt.Errorf("updateCommand: %w", err)
	}

	return nil
}

// attempt carries the command out against the charger. An empty result means "not yet": a
// remote start keeps waiting for the driver to plug in until its deadline.
func (c *controller) attempt(command *entity.Command, now time.Time) (entity.CommandResult, string) {
	if command.ForcedResult != "" {
		return command.ForcedResult, "injected by charger behavior"
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
