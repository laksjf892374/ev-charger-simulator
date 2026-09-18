package scheduler

import (
	"fmt"
	"time"
)

const fakeTickerTimeout = time.Second

type FakeTicker struct {
	Stopped bool
	ticks   chan time.Time
}

func NewFakeTicker() *FakeTicker {
	return &FakeTicker{
		ticks: make(chan time.Time),
	}
}

func (t *FakeTicker) C() <-chan time.Time { return t.ticks }
func (t *FakeTicker) Stop()               { t.Stopped = true }

// Tick delivers one tick and returns once the job goroutine has received it. Because the job
// only receives again after its callback returns, a second Tick returning means the first
// callback has finished.
func (t *FakeTicker) Tick() error {
	select {
	case t.ticks <- time.Time{}:
		return nil
	case <-time.After(fakeTickerTimeout):
		return fmt.Errorf("nobody received the tick within %v", fakeTickerTimeout)
	}
}
