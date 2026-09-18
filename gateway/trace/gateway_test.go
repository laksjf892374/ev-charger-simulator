package trace_test

import (
	"testing"

	"cposim/gateway/trace"
	"cposim/internal/assert"
)

func TestRecord(t *testing.T) {
	t.Run("numbers entries and keeps only the most recent up to capacity", func(t *testing.T) {
		// Given
		traceGateway := trace.NewInMemoryGateway(2)

		// When
		traceGateway.Record(trace.Entry{Summary: "first"})
		traceGateway.Record(trace.Entry{Summary: "second"})
		traceGateway.Record(trace.Entry{Summary: "third"})

		// Then
		assert.Equal(t, traceGateway.ListSince(0), []trace.Entry{
			{Sequence: 2, Summary: "second"},
			{Sequence: 3, Summary: "third"},
		})
	})
}

func TestListSince(t *testing.T) {
	t.Run("returns only entries after the given sequence", func(t *testing.T) {
		// Given
		traceGateway := trace.NewInMemoryGateway(10)
		traceGateway.Record(trace.Entry{Summary: "first"})
		traceGateway.Record(trace.Entry{Summary: "second"})

		// When
		entries := traceGateway.ListSince(1)

		// Then
		assert.Equal(t, entries, []trace.Entry{{Sequence: 2, Summary: "second"}})
	})
}

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
