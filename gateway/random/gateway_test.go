package random_test

import (
	"testing"

	"cposim/gateway/random"
	"cposim/internal/assert"
)

func TestFloat64(t *testing.T) {
	t.Run("returns numbers in [0, 1)", func(t *testing.T) {
		// Given
		randomGateway := random.NewMathGateway()

		// When / Then
		for i := 0; i < 1000; i++ {
			roll := randomGateway.Float64()
			inRange := roll >= 0 && roll < 1
			assert.Equal(t, inRange, true)
		}
	})
}
