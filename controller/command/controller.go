package command

import (
	"fmt"
	"sync"
	"time"

	"cposim/controller/charger"
	"cposim/entity"
	"cposim/gateway/clock"
	"cposim/gateway/events"
	"cposim/gateway/identifier"
	"cposim/gateway/metrics"
	"cposim/gateway/random"
	commandrepo "cposim/repository/command"
)

const commandIDPrefix = "CMD"

type Config struct {
	// Simulated time between accepting a command and the charger acting on it.
	CommandLatency time.Duration
	// How many finished (rejected or resolved) commands are kept; older ones are forgotten.
	MaxFinishedCommands int
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
	metricsGateway    metrics.Gateway
	randomGateway     random.Gateway
	mu                sync.Mutex
}

func NewController(
	chargerController charger.Controller,
	clockGateway clock.Gateway,
	commandRepository commandrepo.Repository,
	config Config,
	eventsGateway events.Gateway,
	identifierGateway identifier.Gateway,
	metricsGateway metrics.Gateway,
	randomGateway random.Gateway,
) (Controller, error) {
	if config.CommandLatency < 0 {
		return nil, fmt.Errorf("command latency must not be negative: latency %v", config.CommandLatency)
	}

	if config.MaxFinishedCommands <= 0 {
		return nil, fmt.Errorf("max finished commands must be positive: max %d", config.MaxFinishedCommands)
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
		metricsGateway:    metricsGateway,
		randomGateway:     randomGateway,
	}, nil
}

func (c *controller) ListCommands() ([]entity.Command, error) {
	commands, err := c.commandRepository.List()
	if err != nil {
		return nil, fmt.Errorf("commandRepository.List: %w", err)
	}

	return commands, nil
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
