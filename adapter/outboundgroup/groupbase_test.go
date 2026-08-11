package outboundgroup

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
)

type urlTestConcurrencyProxy struct {
	C.Proxy
	name    string
	started chan<- struct{}
	release <-chan struct{}
	active  *atomic.Int32
	maximum *atomic.Int32
}

func (p *urlTestConcurrencyProxy) Name() string {
	return p.name
}

func (p *urlTestConcurrencyProxy) URLTest(context.Context, string, utils.IntRanges[uint16]) (uint16, error) {
	running := p.active.Add(1)
	defer p.active.Add(-1)

	for current := p.maximum.Load(); running > current; current = p.maximum.Load() {
		if p.maximum.CompareAndSwap(current, running) {
			break
		}
	}

	p.started <- struct{}{}
	<-p.release
	return 1, nil
}

func TestGroupBaseURLTestLimitsConcurrency(t *testing.T) {
	const proxyCount = urlTestWorkerCount * 2

	started := make(chan struct{}, proxyCount)
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	proxies := make([]C.Proxy, 0, proxyCount)
	for i := 0; i < proxyCount; i++ {
		proxies = append(proxies, &urlTestConcurrencyProxy{
			name:    fmt.Sprintf("proxy-%d", i),
			started: started,
			release: release,
			active:  &active,
			maximum: &maximum,
		})
	}

	group := &GroupBase{
		providerVersions: []uint32{},
		providerProxies:  proxies,
	}
	result := make(chan map[string]uint16, 1)
	go func() {
		delays, err := group.URLTest(context.Background(), "https://example.com", nil)
		if err != nil {
			t.Errorf("URLTest() error = %v", err)
		}
		result <- delays
	}()

	for i := 0; i < urlTestWorkerCount; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker pool did not start all workers")
		}
	}
	select {
	case <-started:
		t.Fatalf("started more than %d concurrent URL tests", urlTestWorkerCount)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	delays := <-result
	if maximum.Load() != urlTestWorkerCount {
		t.Errorf("maximum concurrent URL tests = %d, want %d", maximum.Load(), urlTestWorkerCount)
	}
	if len(delays) != proxyCount {
		t.Errorf("successful delays = %d, want %d", len(delays), proxyCount)
	}
}

type urlTestTimedProxy struct {
	C.Proxy
	name    string
	work    time.Duration
	started *atomic.Int32
}

func (p *urlTestTimedProxy) Name() string {
	return p.name
}

func (p *urlTestTimedProxy) URLTest(ctx context.Context, _ string, _ utils.IntRanges[uint16]) (uint16, error) {
	p.started.Add(1)
	timer := time.NewTimer(p.work)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 1, nil
	}
}

// Worker-pool URLTest must not share one overall deadline across all proxies:
// with concurrency limited, later waves would inherit a nearly-expired budget.
func TestGroupBaseURLTestPerProxyTimeoutAllowsQueuedProxies(t *testing.T) {
	const proxyCount = urlTestWorkerCount + 2
	const perProxy = 40 * time.Millisecond
	const budget = 100 * time.Millisecond

	var started atomic.Int32
	proxies := make([]C.Proxy, 0, proxyCount)
	for i := 0; i < proxyCount; i++ {
		proxies = append(proxies, &urlTestTimedProxy{
			name:    fmt.Sprintf("proxy-%d", i),
			work:    perProxy,
			started: &started,
		})
	}

	group := &GroupBase{
		providerVersions: []uint32{},
		providerProxies:  proxies,
		testTimeout:      int(budget / time.Millisecond),
	}

	// Mimic GET /group/{name}/delay: per-proxy budget, no overall deadline.
	ctx := WithURLTestTimeout(context.Background(), budget)
	delays, err := group.URLTest(ctx, "https://example.com", nil)
	if err != nil {
		t.Fatalf("URLTest() error = %v", err)
	}
	if len(delays) != proxyCount {
		t.Fatalf("successful delays = %d, want %d", len(delays), proxyCount)
	}
	if got := started.Load(); got != int32(proxyCount) {
		t.Fatalf("started = %d, want %d", got, proxyCount)
	}
}

func TestGroupBaseURLTestPropagatesParentCancel(t *testing.T) {
	const proxyCount = urlTestWorkerCount + 2
	const perProxy = time.Second

	var started atomic.Int32
	proxies := make([]C.Proxy, 0, proxyCount)
	for i := 0; i < proxyCount; i++ {
		proxies = append(proxies, &urlTestTimedProxy{
			name:    fmt.Sprintf("proxy-%d", i),
			work:    perProxy,
			started: &started,
		})
	}

	group := &GroupBase{
		providerVersions: []uint32{},
		providerProxies:  proxies,
		testTimeout:      5000,
	}

	ctx, cancel := context.WithCancel(WithURLTestTimeout(context.Background(), perProxy))
	done := make(chan struct{})
	var delays map[string]uint16
	var err error
	go func() {
		delays, err = group.URLTest(ctx, "https://example.com", nil)
		close(done)
	}()

	// Wait until the first wave is in flight, then abort the request.
	deadline := time.Now().Add(time.Second)
	for started.Load() < int32(urlTestWorkerCount) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if started.Load() < int32(urlTestWorkerCount) {
		t.Fatal("workers did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("URLTest did not return after parent cancel")
	}
	if err == nil {
		t.Fatalf("URLTest() error = nil, want timeout after cancel; delays=%v", delays)
	}
	if len(delays) != 0 {
		t.Fatalf("delays = %v, want empty after cancel", delays)
	}
}
