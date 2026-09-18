package clock_test

import (
	"testing"
	"time"

	"cposim/assert"
	"cposim/gateway/clock"
)

var validStartedAt = time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)

func newWallClock(t *testing.T) (clock.NowFunc, func(time.Duration)) {
	t.Helper()

	wallTime := validStartedAt
	wallNow := func() time.Time { return wallTime }
	advanceWallTime := func(duration time.Duration) { wallTime = wallTime.Add(duration) }

	return wallNow, advanceWallTime
}

func TestNewScaledGateway(t *testing.T) {
	t.Run("returns an error when the speed is not positive", func(t *testing.T) {
		// Given
		wallNow, _ := newWallClock(t)

		// When
		_, err := clock.NewScaledGateway(0, wallNow)

		// Then
		assert.Error(t, err)
	})
}

func TestNow(t *testing.T) {
	t.Run("advances simulated time at the configured multiple of wall time", func(t *testing.T) {
		// Given
		wallNow, advanceWallTime := newWallClock(t)
		clockGateway, err := clock.NewScaledGateway(60, wallNow)
		assert.NoError(t, err)

		// When
		advanceWallTime(time.Second)

		// Then
		assert.Equal(t, clockGateway.Now(), validStartedAt.Add(time.Minute))
	})
}

func TestSetSpeed(t *testing.T) {
	t.Run("returns an error when the speed is not positive", func(t *testing.T) {
		// Given
		wallNow, _ := newWallClock(t)
		clockGateway, err := clock.NewScaledGateway(1, wallNow)
		assert.NoError(t, err)

		// When
		err = clockGateway.SetSpeed(-1)

		// Then
		assert.Error(t, err)
		assert.Equal(t, clockGateway.Speed(), 1.0)
	})

	t.Run("applies the new speed only to time that elapses afterwards", func(t *testing.T) {
		// Given
		wallNow, advanceWallTime := newWallClock(t)
		clockGateway, err := clock.NewScaledGateway(1, wallNow)
		assert.NoError(t, err)
		advanceWallTime(10 * time.Second)

		// When
		err = clockGateway.SetSpeed(60)
		advanceWallTime(time.Second)

		// Then
		assert.NoError(t, err)
		assert.Equal(t, clockGateway.Speed(), 60.0)
		assert.Equal(t, clockGateway.Now(), validStartedAt.Add(10*time.Second+time.Minute))
	})
}
