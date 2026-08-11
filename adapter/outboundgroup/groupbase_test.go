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
