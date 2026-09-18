package scheduler_test

import (
	"bytes"
	"errors"
	"testing"

	"cposim/assert"
	"cposim/gateway/scheduler"
)

func newTickerGateway(t *testing.T) (scheduler.Gateway, *scheduler.FakeTicker, *bytes.Buffer) {
	t.Helper()

	fakeTicker := scheduler.NewFakeTicker()
	out := &bytes.Buffer{}
	schedulerGateway := scheduler.NewTickerGateway(
		func() scheduler.Ticker { return fakeTicker },
		out,
	)

	return schedulerGateway, fakeTicker, out
}

func TestStartScheduledJob(t *testing.T) {
	t.Run("returns an error when the job is already scheduled", func(t *testing.T) {
		// Given
		schedulerGateway, _, _ := newTickerGateway(t)
		assert.NoError(t, schedulerGateway.StartScheduledJob("job", func() error { return nil }))

		// When
		err := schedulerGateway.StartScheduledJob("job", func() error { return nil })

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `job "job" is already scheduled`)
		assert.NoError(t, schedulerGateway.StopScheduledJob("job"))
	})

	t.Run("writes callback errors to the output and keeps running", func(t *testing.T) {
		// Given
		schedulerGateway, fakeTicker, out := newTickerGateway(t)
		assert.NoError(t, schedulerGateway.StartScheduledJob("job", func() error { return errors.New("boom") }))

		// When
		assert.NoError(t, fakeTicker.Tick())
		assert.NoError(t, fakeTicker.Tick())
		assert.NoError(t, schedulerGateway.StopScheduledJob("job"))

		// Then
		assert.Contains(t, out.String(), `Error: job "job": boom`)
	})

	t.Run("runs the callback once per tick", func(t *testing.T) {
		// Given
		schedulerGateway, fakeTicker, _ := newTickerGateway(t)
		callbackCount := 0
		assert.NoError(t, schedulerGateway.StartScheduledJob("job", func() error {
			callbackCount++
			return nil
		}))

		// When
		assert.NoError(t, fakeTicker.Tick())
		assert.NoError(t, fakeTicker.Tick())
		assert.NoError(t, schedulerGateway.StopScheduledJob("job"))

		// Then
		assert.Equal(t, callbackCount, 2)
	})
}

func TestStopScheduledJob(t *testing.T) {
	t.Run("returns an error when the job is not scheduled", func(t *testing.T) {
		// Given
		schedulerGateway, _, _ := newTickerGateway(t)

		// When
		err := schedulerGateway.StopScheduledJob("missing")

		// Then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), `job "missing" is not scheduled`)
	})

	t.Run("stops the ticker and delivers no further ticks", func(t *testing.T) {
		// Given
		schedulerGateway, fakeTicker, _ := newTickerGateway(t)
		assert.NoError(t, schedulerGateway.StartScheduledJob("job", func() error { return nil }))

		// When
		err := schedulerGateway.StopScheduledJob("job")

		// Then
		assert.NoError(t, err)
		assert.Equal(t, fakeTicker.Stopped, true)
	})
}
