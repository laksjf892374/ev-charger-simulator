package trace_test

import (
	"testing"

	"cposim/gateway/trace"
	"cposim/internal/assert"
)

func TestFakeGateway(t *testing.T) {
	t.Run("records calls and returns the configured result", func(t *testing.T) {
		// Given
		fakeGateway := trace.NewFakeGateway()
		fakeGateway.ListSinceResult = []trace.Entry{{Summary: "configured"}}

		// When
		fakeGateway.Record(trace.Entry{Summary: "recorded"})
		entries := fakeGateway.ListSince(5)

		// Then
		assert.Equal(t, entries, []trace.Entry{{Summary: "configured"}})
		assert.Equal(t, fakeGateway.ListSinceCalledWith, []int{5})
		assert.Equal(t, fakeGateway.Recorded(), []trace.Entry{{Summary: "recorded"}})
	})
}
