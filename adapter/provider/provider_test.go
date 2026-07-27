package provider

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/metacubex/mihomo/common/yaml"
)

func testSocks5Mapping(name string, port int, dialerProxy string) map[string]any {
	mapping := map[string]any{
		"name":   name,
		"type":   "socks5",
		"server": "127.0.0.1",
		"port":   port,
	}
	if dialerProxy != "" {
		mapping["dialer-proxy"] = dialerProxy
	}
	return mapping
}

func parseProviderProxySet(t *testing.T, filter string, override overrideSchema, payload []map[string]any) (proxySet, error) {
	t.Helper()

	parser, err := NewProxiesParser("provider-a", nil, map[string]struct{}{}, filter, "", "", "", override, "")
	require.NoError(t, err)

	buf, err := yaml.Marshal(ProxySchema{Proxies: payload})
	require.NoError(t, err)

	return parser(buf)
}

func TestNewProxiesParserKeepsFilteredDialerTargetRaw(t *testing.T) {
	prefix := "visible-"
	suffix := "-node"
	tfo := true
	result, err := parseProviderProxySet(t, "^source$", overrideSchema{
		TFO:              &tfo,
		AdditionalPrefix: &prefix,
		AdditionalSuffix: &suffix,
	}, []map[string]any{
		testSocks5Mapping("source", 1080, "hidden"),
		testSocks5Mapping("hidden", 1081, ""),
	})
	require.NoError(t, err)

	require.Len(t, result.proxies, 1)
	source := result.proxies[0]
	require.Equal(t, "visible-source-node", source.Name())
	require.Equal(t, "hidden", source.ProxyInfo().DialerProxy)
	require.True(t, source.ProxyInfo().TFO)

	require.Len(t, result.dialerProxies, 1)
	hidden, ok := result.dialerProxies["hidden"]
	require.True(t, ok)
	require.NotContains(t, result.dialerProxies, "source")
	require.Equal(t, "hidden", hidden.Name())
	require.Empty(t, hidden.ProxyInfo().DialerProxy)
	require.False(t, hidden.ProxyInfo().TFO)
}

func TestNewProxiesParserBuildsRawDialerChain(t *testing.T) {
	result, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
		testSocks5Mapping("source", 1080, "first-hop"),
		testSocks5Mapping("first-hop", 1081, "last-hop"),
		testSocks5Mapping("last-hop", 1082, ""),
	})
	require.NoError(t, err)

	require.Len(t, result.proxies, 1)
	require.Equal(t, "first-hop", result.proxies[0].ProxyInfo().DialerProxy)
	require.Len(t, result.dialerProxies, 2)

	firstHop, ok := result.dialerProxies["first-hop"]
	require.True(t, ok)
	require.Equal(t, "first-hop", firstHop.Name())
	require.Equal(t, "last-hop", firstHop.ProxyInfo().DialerProxy)

	lastHop, ok := result.dialerProxies["last-hop"]
	require.True(t, ok)
	require.Equal(t, "last-hop", lastHop.Name())
	require.Empty(t, lastHop.ProxyInfo().DialerProxy)
}

func TestNewProxiesParserRejectsUnavailableDialerTargets(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "missing target",
			target: "missing",
			want:   "proxy [source] dialer-proxy [missing] not found",
		},
		{
			name:   "other provider target",
			target: "other-provider-target",
			want:   "proxy [source] dialer-proxy [other-provider-target] not found",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
				testSocks5Mapping("source", 1080, test.target),
			})
			require.EqualError(t, err, test.want)
		})
	}
}

func TestNewProxiesParserRejectsReferencedDuplicateRawDialerTarget(t *testing.T) {
	_, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
		testSocks5Mapping("source", 1080, "hidden"),
		testSocks5Mapping("hidden", 1081, ""),
		testSocks5Mapping("hidden", 1082, ""),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hidden")
	require.Contains(t, err.Error(), "duplicate")
}

func TestNewProxiesParserRejectsRawDialerCycle(t *testing.T) {
	_, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
		testSocks5Mapping("source", 1080, "A"),
		testSocks5Mapping("A", 1081, "B"),
		testSocks5Mapping("B", 1082, "A"),
	})
	require.EqualError(t, err, "proxy [A] has circular dialer-proxy dependency")
}

func TestNewProxiesParserPublishesCompleteDialerProxyGenerations(t *testing.T) {
	oldGeneration, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
		testSocks5Mapping("source", 1080, "target"),
		testSocks5Mapping("target", 1081, ""),
	})
	require.NoError(t, err)
	newGeneration, err := parseProviderProxySet(t, "^source$", overrideSchema{}, []map[string]any{
		testSocks5Mapping("source", 1080, "target"),
		testSocks5Mapping("target", 1082, ""),
	})
	require.NoError(t, err)

	oldTarget, ok := oldGeneration.dialerProxies["target"]
	require.True(t, ok)
	newTarget, ok := newGeneration.dialerProxies["target"]
	require.True(t, ok)
	oldAddr := oldTarget.Addr()
	newAddr := newTarget.Addr()

	healthCheck := NewHealthCheck(nil, "", 0, 0, false, nil)
	t.Cleanup(healthCheck.close)
	base := &baseProvider{healthCheck: healthCheck}
	base.setProxies(oldGeneration)

	const readers = 8
	const iterations = 2_000
	start := make(chan struct{})
	errs := make(chan string, readers)
	var waitGroup sync.WaitGroup

	for reader := 0; reader < readers; reader++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for iteration := 0; iteration < iterations; iteration++ {
				proxy, ok := base.GetDialerProxy("target")
				if !ok || proxy == nil {
					errs <- "GetDialerProxy observed an incomplete generation"
					return
				}
				if addr := proxy.Addr(); addr != oldAddr && addr != newAddr {
					errs <- "GetDialerProxy observed a mixed generation"
					return
				}
			}
		}()
	}

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		<-start
		for index := 0; index < iterations; index++ {
			if index%2 == 0 {
				base.setProxies(newGeneration)
			} else {
				base.setProxies(oldGeneration)
			}
		}
	}()

	close(start)
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
