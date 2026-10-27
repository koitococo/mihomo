package adapter

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestURLTestStatisticsRecentTimeouts(t *testing.T) {
	var stats urlTestStatistics
	if got := stats.skipProbability(); got != 0 {
		t.Fatalf("empty probability = %v, want 0", got)
	}
	for i := 0; i < 4; i++ {
		stats.record(context.DeadlineExceeded)
		if got := stats.skipProbability(); got != 0 {
			t.Fatalf("probability after %d timeouts = %v, want 0", i+1, got)
		}
	}
	stats.record(context.DeadlineExceeded)
	if got := stats.skipProbability(); got != 0.95 {
		t.Fatalf("five timeouts probability = %v, want 0.95", got)
	}
	for _, interruption := range []error{nil, errors.New("connection refused"), context.Canceled} {
		stats.record(interruption)
		for i := 0; i < 5; i++ {
			if got := stats.skipProbability(); got != 0 {
				t.Fatalf("interruption %v with %d subsequent timeouts: probability = %v, want 0", interruption, i, got)
			}
			stats.record(context.DeadlineExceeded)
		}
		if got := stats.skipProbability(); got == 0 {
			t.Fatalf("five subsequent timeouts after %v did not restore gate", interruption)
		}
	}
}

func TestURLTestStatisticsCumulativeProbability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		successes int
		want      float64
	}{
		{name: "upper cap", successes: 0, want: 0.95},
		{name: "all actual callers share denominator", successes: 5, want: 0.5},
		{name: "lower cap", successes: 195, want: 0.05},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stats urlTestStatistics
			// Results have no URL or automatic/manual distinction.
			for i := 0; i < tc.successes; i++ {
				stats.record(nil)
			}
			for i := 0; i < 5; i++ {
				stats.record(context.DeadlineExceeded)
			}
			for i := 0; i < 3; i++ {
				if got := stats.skipProbability(); got != tc.want {
					t.Fatalf("probability = %v, want %v", got, tc.want)
				}
			}
			if stats.all != uint64(tc.successes+5) || stats.timeouts != 5 {
				t.Fatalf("probability reads changed actual counts: all=%d timeouts=%d", stats.all, stats.timeouts)
			}
		})
	}
}

type urlTestTimeoutError struct {
	timeout bool
}

func (e urlTestTimeoutError) Error() string   { return "network error" }
func (e urlTestTimeoutError) Timeout() bool   { return e.timeout }
func (e urlTestTimeoutError) Temporary() bool { return false }

func TestIsURLTestTimeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "success"},
		{name: "refused", err: errors.New("connection refused")},
		{name: "status mismatch", err: errors.New("unexpected HTTP status")},
		{name: "canceled", err: context.Canceled},
		{name: "wrapped canceled", err: fmt.Errorf("request: %w", context.Canceled)},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline", err: fmt.Errorf("request: %w", context.DeadlineExceeded), want: true},
		{name: "network timeout", err: urlTestTimeoutError{timeout: true}, want: true},
		{name: "wrapped network timeout", err: fmt.Errorf("request: %w", urlTestTimeoutError{timeout: true}), want: true},
		{name: "network non-timeout", err: urlTestTimeoutError{}},
		{name: "cancellation wins", err: errors.Join(context.Canceled, urlTestTimeoutError{timeout: true})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isURLTestTimeout(tc.err); got != tc.want {
				t.Fatalf("isURLTestTimeout(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
