package random

import "math/rand/v2"

// Gateway is the only source of randomness for the simulation, so that probabilistic behaviors
// stay deterministic in tests.
type Gateway interface {
	// Float64 returns a number in [0, 1).
	Float64() float64
}

type mathGateway struct{}

func NewMathGateway() Gateway {
	return mathGateway{}
}

func (mathGateway) Float64() float64 {
	return rand.Float64()
}
