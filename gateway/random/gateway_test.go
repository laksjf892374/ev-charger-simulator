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

func TestFakeGateway(t *testing.T) {
	t.Run("returns the queued results in order, then the fallback result", func(t *testing.T) {
		// Given
		fakeGateway := random.NewFakeGateway()
		fakeGateway.Float64Result = 0.9
		fakeGateway.Float64Results = []float64{0.1, 0.2}

		// When
		rolls := []float64{fakeGateway.Float64(), fakeGateway.Float64(), fakeGateway.Float64()}

		// Then
		assert.Equal(t, rolls, []float64{0.1, 0.2, 0.9})
		assert.Equal(t, fakeGateway.Float64CallCount, 3)
	})
}
