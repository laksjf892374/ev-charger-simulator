package scheduler_test

import (
	"testing"

	"cposim/gateway/scheduler"
	"cposim/internal/assert"
)

func TestFakeTicker(t *testing.T) {
	t.Run("returns an error when nobody receives the tick in time", func(t *testing.T) {
		// Given
		fakeTicker := scheduler.NewFakeTicker()

		// When
		err := fakeTicker.Tick()

		// Then
		assert.Error(t, err)
	})

	t.Run("delivers a tick to whoever is receiving, and records being stopped", func(t *testing.T) {
		// Given
		fakeTicker := scheduler.NewFakeTicker()
		received := make(chan struct{})
		go func() {
			<-fakeTicker.C()
			close(received)
		}()

		// When
		err := fakeTicker.Tick()
		fakeTicker.Stop()

		// Then
		assert.NoError(t, err)
		<-received
		assert.Equal(t, fakeTicker.Stopped, true)
	})
}
