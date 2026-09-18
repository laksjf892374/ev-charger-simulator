package ocpipush

import (
	"fmt"
	"sync"
	"time"
)

const fakeSenderTimeout = time.Second

type FakeSender struct {
	SendCalledWith []Push
	SendErr        error
	mu             sync.Mutex
	sent           chan struct{}
}

func NewFakeSender() *FakeSender {
	return &FakeSender{
		sent: make(chan struct{}, 1024),
	}
}

func (s *FakeSender) Send(push Push) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SendCalledWith = append(s.SendCalledWith, push)
	s.sent <- struct{}{}

	return s.SendErr
}

// AwaitSends blocks until count further pushes have been sent and returns everything sent so far.
func (s *FakeSender) AwaitSends(count int) ([]Push, error) {
	for i := 0; i < count; i++ {
		select {
		case <-s.sent:
		case <-time.After(fakeSenderTimeout):
			return nil, fmt.Errorf("only %d of %d pushes were sent within %v", i, count, fakeSenderTimeout)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]Push{}, s.SendCalledWith...), nil
}
