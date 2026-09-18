package ocpipush_test

import (
	"errors"
	"testing"

	"cposim/gateway/ocpipush"
	"cposim/internal/assert"
)

func TestFakeSender(t *testing.T) {
	t.Run("returns an error when fewer pushes arrive than awaited", func(t *testing.T) {
		// Given
		fakeSender := ocpipush.NewFakeSender()

		// When
		_, err := fakeSender.AwaitSends(1)

		// Then
		assert.Error(t, err)
	})

	t.Run("records pushes in order and returns the configured error", func(t *testing.T) {
		// Given
		fakeSender := ocpipush.NewFakeSender()
		fakeSender.SendErr = errors.New("boom")

		// When
		firstErr := fakeSender.Send(ocpipush.Push{URL: "first"})
		secondErr := fakeSender.Send(ocpipush.Push{URL: "second"})
		pushes, err := fakeSender.AwaitSends(2)

		// Then
		assert.Error(t, firstErr)
		assert.Error(t, secondErr)
		assert.NoError(t, err)
		assert.Equal(t, pushes, []ocpipush.Push{{URL: "first"}, {URL: "second"}})
	})
}
