package trace

import "sync"

type FakeGateway struct {
	ListSinceCalledWith []int
	ListSinceResult     []Entry
	RecordCalledWith    []Entry
	mu                  sync.Mutex
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{}
}

func (g *FakeGateway) ListSince(sequence int) []Entry {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.ListSinceCalledWith = append(g.ListSinceCalledWith, sequence)

	return g.ListSinceResult
}

func (g *FakeGateway) Record(entry Entry) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.RecordCalledWith = append(g.RecordCalledWith, entry)
}

// Recorded returns a copy that is safe to read while another goroutine is still recording.
func (g *FakeGateway) Recorded() []Entry {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]Entry{}, g.RecordCalledWith...)
}
