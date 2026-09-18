package mockemsp

import (
	"sync"

	"cposim/ocpi"
)

type SendCommandCall struct {
	Kind    string
	Request any
}

type FakeCPOClient struct {
	PullLocationsErr    error
	PullLocationsResult []ocpi.Location
	SendCommandCalls    []SendCommandCall
	SendCommandErr      error
	SendCommandResult   ocpi.CommandResponse
	mu                  sync.Mutex
}

func NewFakeCPOClient() *FakeCPOClient {
	return &FakeCPOClient{}
}

func (c *FakeCPOClient) PullLocations() ([]ocpi.Location, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.PullLocationsResult, c.PullLocationsErr
}

func (c *FakeCPOClient) SendCommand(kind string, request any) (ocpi.CommandResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.SendCommandCalls = append(c.SendCommandCalls, SendCommandCall{
		Kind:    kind,
		Request: request,
	})

	return c.SendCommandResult, c.SendCommandErr
}
