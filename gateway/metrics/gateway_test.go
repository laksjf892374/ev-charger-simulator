package metrics_test

import (
	"testing"

	"cposim/assert"
	"cposim/gateway/metrics"
)

func TestSnapshot(t *testing.T) {
	t.Run("accumulates counters, overwrites gauges, and hands out a copy", func(t *testing.T) {
		// Given
		metricsGateway := metrics.NewInMemoryGateway()

		// When
		metricsGateway.Add(metrics.PushesSent, 2)
		metricsGateway.Add(metrics.PushesSent, 3)
		metricsGateway.Set(metrics.PushQueueDepth, 7)
		metricsGateway.Set(metrics.PushQueueDepth, 4)
		snapshot := metricsGateway.Snapshot()
		snapshot[metrics.PushesSent] = 99

		// Then
		assert.Equal(t, metricsGateway.Snapshot(), map[string]int64{
			metrics.PushQueueDepth: 4,
			metrics.PushesSent:     5,
		})
	})
}
