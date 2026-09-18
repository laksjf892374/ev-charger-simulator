// Package app is the dependency-injection root: the only place that constructs concrete
// implementations and holds the tunable constants.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"cposim/gateway/clock"
	"cposim/gateway/metrics"
	"cposim/gateway/random"
	"cposim/gateway/scheduler"
)

const (
	HealthPath = "/healthz"
	ResetPath  = "/api/reset"

	// Wall time. The simulation is unhealthy once it has not ticked for this long: requests would
	// still be answered, but time would have stopped.
	maxTickAge        = 5 * time.Second
	simulationJobID   = "simulation"
	tickFrequencyWall = 250 * time.Millisecond
)

type Config struct {
	// Where OCPI pushes are delivered: the base URL of an eMSP's receiver endpoints.
	EMSPBaseURL string
	// How fast simulated time runs when a world is created, as a multiple of wall time. Zero
	// means real time.
	InitialSpeed float64
	// When set, the bundled mock eMSP is mounted in this process. It is this server's own base
	// URL: the mock reaches the CPO through it, and gives it to the CPO as its callback address.
	MockEMSPSelfBaseURL string
	Out                 io.Writer
}

type Simulator interface {
	ConnectMockEMSP() error
	Handler() http.Handler
	Reset() error
	Seed() error
	Start() error
	Stop() error
}

// simulator owns what outlives any one world: the HTTP entry point, the tick, logging, metrics
// and the ability to replace the world.
type simulator struct {
	config            Config
	currentWorld      atomic.Pointer[world]
	handler           http.Handler
	lastTickWallNanos atomic.Int64
	logger            *slog.Logger
	metricsGateway    metrics.Gateway
	randomGateway     random.Gateway
	resetMu           sync.Mutex
	schedulerGateway  scheduler.Gateway
	wallNow           clock.NowFunc
	worldCount        int
	mu                sync.Mutex
}

func NewSimulator(config Config) (Simulator, error) {
	return NewSimulatorWithTicker(
		config,
		scheduler.NewRealTickerFunc(tickFrequencyWall),
		random.NewMathGateway(),
		time.Now,
	)
}

func NewSimulatorWithTicker(
	config Config,
	newTicker scheduler.NewTickerFunc,
	randomGateway random.Gateway,
	wallNow clock.NowFunc,
) (Simulator, error) {
	// Everything the process reports (requests, failed ticks, failed or dropped pushes) is one
	// structured log, written as JSON lines. slog serialises concurrent writers itself.
	logger := slog.New(slog.NewJSONHandler(config.Out, nil))

	s := &simulator{
		config:           config,
		logger:           logger,
		metricsGateway:   metrics.NewInMemoryGateway(),
		randomGateway:    randomGateway,
		schedulerGateway: scheduler.NewTickerGateway(logger, newTicker),
		wallNow:          wallNow,
	}
	s.lastTickWallNanos.Store(wallNow().UnixNano())

	if _, err := s.replaceWorld(); err != nil {
		return nil, fmt.Errorf("replaceWorld: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+HealthPath, s.getHealth)
	mux.HandleFunc("POST "+ResetPath, s.postReset)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.currentWorld.Load().handler.ServeHTTP(w, r)
	})
	s.handler = s.logged(mux)

	return s, nil
}

// replaceWorld builds an empty world and makes it current. The previous world, if any, is
// returned still running, so the caller decides when to stop it.
func (s *simulator) replaceWorld() (*world, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.worldCount++

	next, err := newWorld(
		s.config,
		s.logger,
		s.metricsGateway,
		s.randomGateway,
		s.wallNow,
		fmt.Sprintf("WORLD-%06d", s.worldCount),
	)
	if err != nil {
		return nil, fmt.Errorf("newWorld: %w", err)
	}

	return s.currentWorld.Swap(next), nil
}

func (s *simulator) Handler() http.Handler {
	return s.handler
}

func (s *simulator) Seed() error {
	if err := s.currentWorld.Load().seed(); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	return nil
}

func (s *simulator) ConnectMockEMSP() error {
	if err := s.currentWorld.Load().connectMockEMSP(); err != nil {
		return fmt.Errorf("connectMockEMSP: %w", err)
	}

	return nil
}

// Reset replaces the world with a freshly seeded one. The new world becomes current before it is
// seeded, because seeding pushes to the eMSP, and the bundled mock eMSP is reached through this
// very server: the pushes must land in the new world's mock, not the old one's.
func (s *simulator) Reset() error {
	// one reset at a time, start to finish: two interleaved resets would seed the same world twice
	s.resetMu.Lock()
	defer s.resetMu.Unlock()

	previous, err := s.replaceWorld()
	if err != nil {
		return fmt.Errorf("replaceWorld: %w", err)
	}

	previous.pushGateway.Stop()

	if err := s.Seed(); err != nil {
		return fmt.Errorf("Seed: %w", err)
	}

	if err := s.ConnectMockEMSP(); err != nil {
		return fmt.Errorf("ConnectMockEMSP: %w", err)
	}

	s.metricsGateway.Add(metrics.WorldResets, 1)

	return nil
}

func (s *simulator) postReset(w http.ResponseWriter, r *http.Request) {
	if err := s.Reset(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Reset: %v", err)})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"world_id": s.currentWorld.Load().worldID})
}

func respondJSON(w http.ResponseWriter, httpStatus int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)

	// an encode failure here means the client went away; there is nobody left to tell
	_ = json.NewEncoder(w).Encode(body)
}

type healthView struct {
	LastTickAgeMillis int64  `json:"last_tick_age_ms"`
	Status            string `json:"status"`
	WorldID           string `json:"world_id"`
}

// getHealth reports on the simulation, not just the HTTP server: a process whose tick has died
// still answers requests, and would otherwise look healthy forever.
func (s *simulator) getHealth(w http.ResponseWriter, r *http.Request) {
	lastTickAge := s.wallNow().Sub(time.Unix(0, s.lastTickWallNanos.Load()))
	health := healthView{
		LastTickAgeMillis: lastTickAge.Milliseconds(),
		Status:            "ok",
		WorldID:           s.currentWorld.Load().worldID,
	}

	if lastTickAge > maxTickAge {
		health.Status = "simulation clock has stopped"
		respondJSON(w, http.StatusServiceUnavailable, health)
		return
	}

	respondJSON(w, http.StatusOK, health)
}

func (s *simulator) Start() error {
	if err := s.schedulerGateway.StartScheduledJob(simulationJobID, s.tick); err != nil {
		return fmt.Errorf("schedulerGateway.StartScheduledJob: %w", err)
	}

	return nil
}

func (s *simulator) tick() error {
	startedAt := s.wallNow()
	err := s.currentWorld.Load().tick()
	finishedAt := s.wallNow()

	s.lastTickWallNanos.Store(finishedAt.UnixNano())
	s.metricsGateway.Add(metrics.Ticks, 1)
	s.metricsGateway.Set(metrics.LastTickDurationMicros, finishedAt.Sub(startedAt).Microseconds())

	if err != nil {
		s.metricsGateway.Add(metrics.TickErrors, 1)
		return fmt.Errorf("tick: %w", err)
	}

	return nil
}

func (s *simulator) Stop() error {
	if err := s.schedulerGateway.StopScheduledJob(simulationJobID); err != nil {
		return fmt.Errorf("schedulerGateway.StopScheduledJob: %w", err)
	}

	s.currentWorld.Load().pushGateway.Stop()

	return nil
}
