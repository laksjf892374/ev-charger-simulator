package clock

import (
	"sync"
	"time"
)

type FakeGateway struct {
	NowResult          time.Time
	SetSpeedCalledWith []float64
	SetSpeedErr        error
	SpeedResult        float64
	mu                 sync.Mutex
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{
		NowResult:   time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC),
		SpeedResult: 1,
	}
}

func (g *FakeGateway) Advance(duration time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.NowResult = g.NowResult.Add(duration)
}

func (g *FakeGateway) Now() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.NowResult
}

func (g *FakeGateway) SetSpeed(speed float64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.SetSpeedCalledWith = append(g.SetSpeedCalledWith, speed)

	return g.SetSpeedErr
}

func (g *FakeGateway) Speed() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.SpeedResult
}
