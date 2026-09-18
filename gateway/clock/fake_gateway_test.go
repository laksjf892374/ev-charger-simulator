package clock_test

import (
	"errors"
	"testing"
	"time"

	"cposim/gateway/clock"
	"cposim/internal/assert"
)

func TestFakeGateway(t *testing.T) {
	t.Run("returns the configured time and moves it forward on Advance", func(t *testing.T) {
		// Given
		fakeGateway := clock.NewFakeGateway()
		startedAt := fakeGateway.Now()

		// When
		fakeGateway.Advance(time.Minute)

		// Then
		assert.Equal(t, fakeGateway.Now(), startedAt.Add(time.Minute))
	})

	t.Run("records SetSpeed calls and returns the configured error", func(t *testing.T) {
		// Given
		fakeGateway := clock.NewFakeGateway()
		fakeGateway.SetSpeedErr = errors.New("boom")

		// When
		err := fakeGateway.SetSpeed(10)

		// Then
		assert.Error(t, err)
		assert.Equal(t, fakeGateway.SetSpeedCalledWith, []float64{10})
		assert.Equal(t, fakeGateway.Speed(), 1.0)
	})
}
