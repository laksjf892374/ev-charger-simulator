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
