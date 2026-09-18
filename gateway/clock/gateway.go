package clock

import (
	"fmt"
	"sync"
	"time"
)

// Gateway is the only source of time for the simulation. Simulated time runs at Speed() times
// wall time, so a 40-minute charge can be watched in 40 seconds.
type Gateway interface {
	Now() time.Time
	SetSpeed(speed float64) error
	Speed() float64
}

type NowFunc func() time.Time

type scaledGateway struct {
	simulatedAnchor time.Time
	speed           float64
	wallAnchor      time.Time
	wallNow         NowFunc
	mu              sync.Mutex
}

func NewScaledGateway(
	speed float64,
	wallNow NowFunc,
) (Gateway, error) {
	if speed <= 0 {
		return nil, fmt.Errorf("speed must be positive: speed %v", speed)
	}

	startedAt := wallNow().UTC()

	return &scaledGateway{
		simulatedAnchor: startedAt,
		speed:           speed,
		wallAnchor:      startedAt,
		wallNow:         wallNow,
	}, nil
}

func (g *scaledGateway) Now() time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.now()
}

func (g *scaledGateway) now() time.Time {
	wallElapsed := g.wallNow().Sub(g.wallAnchor)
	simulatedElapsed := time.Duration(float64(wallElapsed) * g.speed)

	return g.simulatedAnchor.Add(simulatedElapsed)
}

func (g *scaledGateway) SetSpeed(speed float64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if speed <= 0 {
		return fmt.Errorf("speed must be positive: speed %v", speed)
	}

	// re-anchor so time already elapsed keeps the speed it elapsed at
	g.simulatedAnchor = g.now()
	g.wallAnchor = g.wallNow()
	g.speed = speed

	return nil
}

func (g *scaledGateway) Speed() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	return g.speed
}
