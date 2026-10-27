package outboundgroup

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

// groupHealthCheck owns only this group's automatic checks. Provider checks and
// manual URL tests intentionally do not share its running or lazy state.
type groupHealthCheck struct {
	interval       time.Duration
	lazy           bool
	lastTouch      atomic.Int64
	running        atomic.Bool
	cancel         context.CancelFunc
	url            string
	expectedStatus utils.IntRanges[uint16]
}

func (hc *groupHealthCheck) touch(now time.Time) {
	hc.lastTouch.Store(now.UnixNano())
}

func (hc *groupHealthCheck) begin(ctx context.Context, now time.Time, initial bool) (start, busy bool) {
	if ctx.Err() != nil {
		return false, false
	}
	if hc.running.Load() {
		return false, true
	}
	if !initial && hc.lazy && now.Sub(time.Unix(0, hc.lastTouch.Load())) >= hc.interval {
		return false, false
	}
	return hc.running.CompareAndSwap(false, true), false
}

func (gb *GroupBase) configureHealthCheck(option GroupCommonOption, expectedStatus utils.IntRanges[uint16]) {
	gb.groupHealthCheck = &groupHealthCheck{
		interval:       time.Duration(option.Interval) * time.Second,
		lazy:           option.Lazy,
		url:            option.URL,
		expectedStatus: expectedStatus,
	}
}

// StartHealthCheck activates the group's ticker after its providers are loaded.
func (gb *GroupBase) StartHealthCheck() {
	hc := gb.groupHealthCheck
	if hc == nil || hc.interval <= 0 || hc.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel
	ticker := time.NewTicker(hc.interval)
	start := func(initial bool) {
		ok, busy := hc.begin(ctx, time.Now(), initial)
		if busy {
			log.Infoln("Skip health check for group [%s]: previous check is still running", gb.Name())
		}
		if !ok {
			return
		}
		go func() {
			defer hc.running.Store(false)
			_, _ = gb.urlTest(ctx, hc.url, hc.expectedStatus, true)
		}()
	}
	start(true)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				start(false)
			}
		}
	}()
}

// StopHealthCheck stops future triggers and cancels any in-flight group check.
func (gb *GroupBase) StopHealthCheck() {
	if hc := gb.groupHealthCheck; hc != nil && hc.cancel != nil {
		hc.cancel()
	}
}

func shouldSkipAutomaticURLTest(proxy C.Proxy, sample float64) bool {
	stats, ok := proxy.(interface{ AutomaticURLTestSkipProbability() float64 })
	return ok && sample < stats.AutomaticURLTestSkipProbability()
}
