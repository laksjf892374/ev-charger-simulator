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
	maxSpeed        float64
	simulatedAnchor time.Time
	speed           float64
	wallAnchor      time.Time
	wallNow         NowFunc
	mu              sync.Mutex
}

func NewScaledGateway(
	maxSpeed float64,
	speed float64,
	wallNow NowFunc,
) (Gateway, error) {
	if err := validateSpeed(speed, maxSpeed); err != nil {
		return nil, fmt.Errorf("validateSpeed: %w", err)
	}

	startedAt := wallNow().UTC()

	return &scaledGateway{
		maxSpeed:        maxSpeed,
		simulatedAnchor: startedAt,
		speed:           speed,
		wallAnchor:      startedAt,
		wallNow:         wallNow,
	}, nil
}

// An unbounded speed would turn one tick into years of simulated time.
func validateSpeed(speed float64, maxSpeed float64) error {
	if speed <= 0 || speed > maxSpeed {
		return fmt.Errorf("speed must be above 0 and at most %v: speed %v", maxSpeed, speed)
	}

	return nil
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

	if err := validateSpeed(speed, g.maxSpeed); err != nil {
		return fmt.Errorf("validateSpeed: %w", err)
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
