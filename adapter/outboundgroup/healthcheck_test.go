package outboundgroup

import (
	"context"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

func TestGroupHealthCheckSkipsRunningTicks(t *testing.T) {
	hc := &groupHealthCheck{interval: time.Second}
	ctx := context.Background()
	now := time.Unix(100, 0)
	if start, busy := hc.begin(ctx, now, true); !start || busy {
		t.Fatal("initial check did not start")
	}
	for i := 1; i <= 3; i++ {
		if start, busy := hc.begin(ctx, now.Add(time.Duration(i)*time.Second), false); start || !busy {
			t.Fatal("running check did not skip a tick")
		}
	}
	// Completion only releases the running gate: skipped ticks are not queued.
	hc.running.Store(false)
	if start, busy := hc.begin(ctx, now.Add(4*time.Second), false); !start || busy {
		t.Fatal("next tick after completion did not start")
	}
}

func TestGroupHealthChecksHaveIndependentRunningState(t *testing.T) {
	// Scheduling state belongs to groups, not their shared providers or URL.
	first := &groupHealthCheck{interval: time.Second, url: "https://example.com"}
	second := &groupHealthCheck{interval: time.Second, url: first.url}
	ctx := context.Background()
	now := time.Unix(100, 0)
	if start, _ := first.begin(ctx, now, true); !start {
		t.Fatal("first group did not start")
	}
	if start, busy := second.begin(ctx, now, true); !start || busy {
		t.Fatal("first group blocked the second group")
	}
	first.running.Store(false)
	if start, busy := second.begin(ctx, now.Add(time.Second), false); start || !busy {
		t.Fatal("completing first group released the second group's gate")
	}
}

func TestGroupHealthCheckCancellation(t *testing.T) {
	hc := &groupHealthCheck{interval: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel
	group := &GroupBase{groupHealthCheck: hc}
	group.StopHealthCheck()
	if start, busy := hc.begin(ctx, time.Unix(100, 0), true); start || busy {
		t.Fatal("stopped group accepted an initial trigger")
	}
	if start, busy := hc.begin(ctx, time.Unix(101, 0), false); start || busy {
		t.Fatal("stopped group accepted a periodic trigger")
	}
}

func TestGroupHealthCheckLazyTouchIsLocal(t *testing.T) {
	now := time.Unix(100, 0)
	first := &groupHealthCheck{interval: time.Second, lazy: true}
	second := &groupHealthCheck{interval: time.Second, lazy: true}
	ctx := context.Background()
	if start, _ := first.begin(ctx, now, true); !start {
		t.Fatal("lazy group skipped its initial check")
	}
	first.running.Store(false)
	if start, _ := first.begin(ctx, now.Add(time.Second), false); start {
		t.Fatal("untouched lazy group started")
	}
	first.touch(now)
	if start, _ := first.begin(ctx, now.Add(time.Second/2), false); !start {
		t.Fatal("recently touched group did not start")
	}
	if start, _ := second.begin(ctx, now.Add(time.Second/2), false); start {
		t.Fatal("touching one group activated another group")
	}
	first.running.Store(false)
	if start, _ := first.begin(ctx, now.Add(time.Second), false); start {
		t.Fatal("expired touch activated a lazy group")
	}
}

type groupHealthCheckCancelProxy struct {
	C.Proxy
	started chan struct{}
	stopped chan struct{}
}

func (p *groupHealthCheckCancelProxy) URLTest(ctx context.Context, _ string, _ utils.IntRanges[uint16]) (uint16, error) {
	close(p.started)
	<-ctx.Done()
	close(p.stopped)
	return 0, ctx.Err()
}

func TestGroupHealthCheckStopCancelsActiveWorker(t *testing.T) {
	proxy := &groupHealthCheckCancelProxy{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	group := &GroupBase{
		groupHealthCheck: &groupHealthCheck{interval: time.Hour},
		testTimeout:      int(time.Hour / time.Millisecond),
		providerVersions: []uint32{},
		providerProxies:  []C.Proxy{proxy},
	}
	group.StartHealthCheck()
	defer group.StopHealthCheck()
	select {
	case <-proxy.started:
	case <-time.After(time.Second):
		t.Fatal("initial check did not start")
	}
	group.StopHealthCheck()
	select {
	case <-proxy.stopped:
	case <-time.After(time.Second):
		t.Fatal("stopping the group did not cancel its active worker")
	}
}

type groupStatusProvider struct {
	P.ProxyProvider
	url    string
	status utils.IntRanges[uint16]
}

func (p *groupStatusProvider) HealthCheckURL() string                             { return p.url }
func (p *groupStatusProvider) HealthCheckExpectedStatus() utils.IntRanges[uint16] { return p.status }

func TestGroupExpectedStatusInheritance(t *testing.T) {
	providerStatus, _ := utils.NewUnsignedRanges[uint16]("204")
	explicitStatus, _ := utils.NewUnsignedRanges[uint16]("200")
	providers := []P.ProxyProvider{&groupStatusProvider{url: "https://provider.test", status: providerStatus}}
	for _, test := range []struct {
		name       string
		url        string
		explicit   bool
		configured utils.IntRanges[uint16]
		want       string
	}{
		{name: "matching provider", url: "https://provider.test", want: "204"},
		{name: "different URL", url: "https://group.test", want: "*"},
		{name: "explicit status wins", url: "https://provider.test", explicit: true, configured: explicitStatus, want: "200"},
		{name: "explicit wildcard wins", url: "https://provider.test", explicit: true, want: "*"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := groupExpectedStatus(test.url, test.configured, test.explicit, providers)
			if got.String() != test.want {
				t.Fatalf("expected status = %s, want %s", got.String(), test.want)
			}
		})
	}
}

type groupRecoveryProxy struct {
	C.Proxy
	url    string
	status utils.IntRanges[uint16]
	calls  int
}

func (p *groupRecoveryProxy) Name() string { return "recovery-member" }
func (p *groupRecoveryProxy) URLTest(_ context.Context, url string, status utils.IntRanges[uint16]) (uint16, error) {
	p.url, p.status = url, status
	p.calls++
	return 1, nil
}

func TestGroupFailureRecoveryChecksGroupURLWithAutoDisabled(t *testing.T) {
	proxy := &groupRecoveryProxy{}
	group := NewGroupBase(GroupBaseOption{TestTimeout: 5000})
	group.providerVersions = []uint32{}
	group.providerProxies = []C.Proxy{proxy}
	status, _ := utils.NewUnsignedRanges[uint16]("204")
	group.configureHealthCheck(GroupCommonOption{URL: "https://group.test", Interval: 0}, status)
	group.failedTimes = 5
	group.healthCheck()
	if proxy.calls != 1 || proxy.url != "https://group.test" || proxy.status.String() != "204" {
		t.Fatalf("recovery check: calls=%d URL=%s status=%s", proxy.calls, proxy.url, proxy.status.String())
	}
	if group.failedTesting.Load() || group.failedTimes != 0 {
		t.Fatal("recovery did not release failed-testing state")
	}
}
