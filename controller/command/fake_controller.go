package command

import (
	"sync"

	"cposim/entity"
)

type FakeController struct {
	ListCommandsErr           error
	ListCommandsResult        []entity.Command
	StartSessionCalledWith    []StartSessionInput
	StartSessionErr           error
	StartSessionResult        entity.Command
	StopSessionCalledWith     []StopSessionInput
	StopSessionErr            error
	StopSessionResult         entity.Command
	TickCallCount             int
	TickErr                   error
	UnlockConnectorCalledWith []UnlockConnectorInput
	UnlockConnectorErr        error
	UnlockConnectorResult     entity.Command
	mu                        sync.Mutex
}

func NewFakeController() *FakeController {
	return &FakeController{}
}

func (c *FakeController) ListCommands() ([]entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ListCommandsResult, c.ListCommandsErr
}

func (c *FakeController) StartSession(input StartSessionInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StartSessionCalledWith = append(c.StartSessionCalledWith, input)

	return c.StartSessionResult, c.StartSessionErr
}

func (c *FakeController) StopSession(input StopSessionInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StopSessionCalledWith = append(c.StopSessionCalledWith, input)

	return c.StopSessionResult, c.StopSessionErr
}

func (c *FakeController) Tick() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.TickCallCount++

	return c.TickErr
}

func (c *FakeController) UnlockConnector(input UnlockConnectorInput) (entity.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.UnlockConnectorCalledWith = append(c.UnlockConnectorCalledWith, input)

	return c.UnlockConnectorResult, c.UnlockConnectorErr
}
