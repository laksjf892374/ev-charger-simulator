package random

import "sync"

// FakeGateway returns Float64Results in order, then Float64Result forever.
type FakeGateway struct {
	Float64CallCount int
	Float64Result    float64
	Float64Results   []float64
	mu               sync.Mutex
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{}
}

func (g *FakeGateway) Float64() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Float64CallCount++

	if len(g.Float64Results) == 0 {
		return g.Float64Result
	}

	result := g.Float64Results[0]
	g.Float64Results = g.Float64Results[1:]

	return result
}
