package adapter

import (
	"context"
	"errors"
	"net"
	"sync"
)

// urlTestStatistics tracks actual completions across all URLs and callers.
// The low five bits hold the most recent results, with one denoting timeout.
type urlTestStatistics struct {
	mu       sync.Mutex
	all      uint64
	timeouts uint64
	recent   uint8
}

func (s *urlTestStatistics) record(err error) {
	timedOut := isURLTestTimeout(err)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all++
	s.recent = (s.recent << 1) & 0x1f
	if timedOut {
		s.timeouts++
		s.recent |= 1
	}
}

func (s *urlTestStatistics) skipProbability() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.all < 5 || s.recent != 0x1f {
		return 0
	}
	probability := float64(s.timeouts) / float64(s.all)
	if probability < 0.05 {
		return 0.05
	}
	if probability > 0.95 {
		return 0.95
	}
	return probability
}

func isURLTestTimeout(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
