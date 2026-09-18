package trace

import (
	"sync"
	"time"
)

const (
	DirectionInbound  = "EMSP_TO_CPO"
	DirectionOutbound = "CPO_TO_EMSP"

	ModuleCDRs      = "cdrs"
	ModuleCommands  = "commands"
	ModuleLocations = "locations"
	ModuleSessions  = "sessions"
	ModuleVersions  = "versions"
)

// Entry is one OCPI exchange as seen from the CPO, with a plain-English summary so that someone
// who has never read the OCPI spec can follow along.
type Entry struct {
	Direction string `json:"direction"`
	Method    string `json:"method"`
	// The OCPI module the exchange belongs to: locations, sessions, cdrs, commands or versions.
	Module       string    `json:"module"`
	RecordedAt   time.Time `json:"recorded_at"`
	RequestBody  string    `json:"request_body,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
	Sequence     int       `json:"sequence"`
	StatusCode   int       `json:"status_code"`
	Summary      string    `json:"summary"`
	URL          string    `json:"url"`
}

type Gateway interface {
	ListSince(sequence int) []Entry
	Record(entry Entry)
}

type inMemoryGateway struct {
	capacity     int
	entries      []Entry
	lastSequence int
	mu           sync.Mutex
}

// NewInMemoryGateway keeps the most recent capacity entries.
func NewInMemoryGateway(capacity int) Gateway {
	return &inMemoryGateway{
		capacity: capacity,
	}
}

// ListSince returns entries with a sequence greater than the given one, oldest first.
func (g *inMemoryGateway) ListSince(sequence int) []Entry {
	g.mu.Lock()
	defer g.mu.Unlock()

	entries := []Entry{}
	for _, entry := range g.entries {
		if entry.Sequence > sequence {
			entries = append(entries, entry)
		}
	}

	return entries
}

func (g *inMemoryGateway) Record(entry Entry) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastSequence++
	entry.Sequence = g.lastSequence

	g.entries = append(g.entries, entry)
	if len(g.entries) > g.capacity {
		g.entries = g.entries[len(g.entries)-g.capacity:]
	}
}
