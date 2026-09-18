package metrics

import "sync"

const (
	CDRsKept               = "cdrs_kept"
	Chargers               = "chargers"
	ChargersInStatePrefix  = "chargers_"
	CommandsPending        = "commands_pending"
	CommandsRejected       = "commands_rejected_total"
	CommandsResolvedPrefix = "commands_resolved_total_"
	HTTPRequests           = "http_requests_total"
	HTTPServerErrors       = "http_server_errors_total"
	LastTickDurationMicros = "last_tick_duration_micros"
	SessionsActive         = "sessions_active"
	SessionsKept           = "sessions_kept"
	Sites                  = "sites"
	PushQueueDepth         = "push_queue_depth"
	PushesDropped          = "pushes_dropped_total"
	PushesFailed           = "pushes_failed_total"
	PushesSent             = "pushes_sent_total"
	TickErrors             = "tick_errors_total"
	Ticks                  = "ticks_total"
	WorldResets            = "world_resets_total"
)

// Gateway collects process-wide counters and gauges. It is deliberately tiny: named int64s that
// the control API serves as JSON, which is enough to see whether the simulator is healthy and
// what it has been doing without adding a dependency.
type Gateway interface {
	Add(name string, delta int64)
	Set(name string, value int64)
	Snapshot() map[string]int64
}

type inMemoryGateway struct {
	valueByName map[string]int64
	mu          sync.Mutex
}

func NewInMemoryGateway() Gateway {
	return &inMemoryGateway{
		valueByName: map[string]int64{},
	}
}

func (g *inMemoryGateway) Add(name string, delta int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.valueByName[name] += delta
}

func (g *inMemoryGateway) Set(name string, value int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.valueByName[name] = value
}

func (g *inMemoryGateway) Snapshot() map[string]int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	snapshot := make(map[string]int64, len(g.valueByName))
	for name, value := range g.valueByName {
		snapshot[name] = value
	}

	return snapshot
}
