package scheduler

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Gateway interface {
	StartScheduledJob(jobID string, callback func() error) error
	StopScheduledJob(jobID string) error
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type NewTickerFunc func() Ticker

type job struct {
	stop    chan struct{}
	stopped chan struct{}
}

type tickerGateway struct {
	jobByJobID map[string]job
	logger     *slog.Logger
	newTicker  NewTickerFunc
	mu         sync.Mutex
}

func NewTickerGateway(
	logger *slog.Logger,
	newTicker NewTickerFunc,
) Gateway {
	return &tickerGateway{
		jobByJobID: map[string]job{},
		logger:     logger,
		newTicker:  newTicker,
	}
}

func NewRealTickerFunc(interval time.Duration) NewTickerFunc {
	return func() Ticker {
		return realTicker{ticker: time.NewTicker(interval)}
	}
}

type realTicker struct {
	ticker *time.Ticker
}

func (t realTicker) C() <-chan time.Time { return t.ticker.C }
func (t realTicker) Stop()               { t.ticker.Stop() }

func (g *tickerGateway) StartScheduledJob(jobID string, callback func() error) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.jobByJobID[jobID]; ok {
		return fmt.Errorf("job %q is already scheduled", jobID)
	}

	scheduledJob := job{
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	g.jobByJobID[jobID] = scheduledJob

	go g.run(jobID, scheduledJob, callback)

	return nil
}

func (g *tickerGateway) run(jobID string, scheduledJob job, callback func() error) {
	defer close(scheduledJob.stopped)

	ticker := g.newTicker()
	defer ticker.Stop()

	for {
		select {
		case <-scheduledJob.stop:
			return
		case <-ticker.C():
			// a background job has nobody to return an error to
			if err := runCallback(callback); err != nil {
				g.logger.Error("scheduled job failed", "job_id", jobID, "error", err.Error())
			}
		}
	}
}

// runCallback turns a panic into an error. One bad tick must not end the job: a simulation whose
// clock has silently stopped is worse than one that logged an error and carried on.
func runCallback(callback func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("callback panicked: %v", recovered)
		}
	}()

	return callback()
}

// StopScheduledJob blocks until the job goroutine has exited, so no callback runs after it
// returns. It must not be called from inside the job's own callback.
func (g *tickerGateway) StopScheduledJob(jobID string) error {
	g.mu.Lock()
	scheduledJob, ok := g.jobByJobID[jobID]
	delete(g.jobByJobID, jobID)
	g.mu.Unlock()

	if !ok {
		return fmt.Errorf("job %q is not scheduled", jobID)
	}

	close(scheduledJob.stop)
	<-scheduledJob.stopped

	return nil
}
