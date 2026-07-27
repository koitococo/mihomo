package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDialerProxies(t *testing.T) {
	testCases := []struct {
		testName string
		proxy    []map[string]any
		// the proxy named in a cycle error depends on map iteration order,
		// so any of these errors is accepted; nil means no error
		errs []string
	}{
		{
			testName: "ValidReference",
			proxy: []map[string]any{ // create proxy with valid dialer-proxy reference
				{"name": "base-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "proxy-with-dialer", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "base-proxy"},
			},
		},
		{
			testName: "NotFoundReference",
			proxy: []map[string]any{ // create proxy with non-existent dialer-proxy reference
				{"name": "base-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "valid-proxy-with-dialer", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "base-proxy"},
				{"name": "proxy-with-dialer", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "non-existent-proxy"},
			},
			errs: []string{"proxy [proxy-with-dialer] dialer-proxy [non-existent-proxy] not found"},
		},
		{
			testName: "CircularDependency",
			proxy: []map[string]any{ // proxy-b -> proxy-a <-> proxy-c
				{"name": "proxy-a", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "proxy-c"},
				{"name": "proxy-b", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "proxy-a"},
				{"name": "proxy-c", "type": "socks5", "server": "127.0.0.1", "port": 1082, "dialer-proxy": "proxy-a"},
			},
			errs: []string{
				"proxy [proxy-a] has circular dialer-proxy dependency",
				"proxy [proxy-b] has circular dialer-proxy dependency",
				"proxy [proxy-c] has circular dialer-proxy dependency",
			},
		},
		{
			testName: "ComplexChain",
			proxy: []map[string]any{ // proxy-d, proxy-e and proxy-f share the chain proxy-c -> proxy-b -> proxy-a
				{"name": "proxy-a", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "proxy-b", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "proxy-a"},
				{"name": "proxy-c", "type": "socks5", "server": "127.0.0.1", "port": 1082, "dialer-proxy": "proxy-b"},
				{"name": "proxy-d", "type": "socks5", "server": "127.0.0.1", "port": 1083, "dialer-proxy": "proxy-c"},
				{"name": "proxy-e", "type": "socks5", "server": "127.0.0.1", "port": 1084, "dialer-proxy": "proxy-c"},
				{"name": "proxy-f", "type": "socks5", "server": "127.0.0.1", "port": 1085, "dialer-proxy": "proxy-c"},
			},
		},
		{
			testName: "EmptyDialerProxy",
			proxy: []map[string]any{ // create proxy without dialer-proxy
				{"name": "simple-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "another-simple-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1081},
			},
		},
		{
			testName: "SelfReference",
			proxy: []map[string]any{ // create proxy that references itself
				{"name": "base-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "self-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "self-proxy"},
			},
			errs: []string{"proxy [self-proxy] has circular dialer-proxy dependency"},
		},
		{
			testName: "NotFoundBeforeCycle",
			proxy: []map[string]any{
				{"name": "a", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "b"},
				{"name": "b", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "a"},
				{"name": "c", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "missing"},
			},
			errs: []string{"proxy [c] dialer-proxy [missing] not found"},
		},
		{
			testName: "TwoProxyCycle",
			proxy: []map[string]any{
				{"name": "a", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "b"},
				{"name": "b", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "a"},
				{"name": "c", "type": "socks5", "server": "127.0.0.1", "port": 1080},
			},
			errs: []string{
				"proxy [a] has circular dialer-proxy dependency",
				"proxy [b] has circular dialer-proxy dependency",
			},
		},
		{
			testName: "LongChainIntoCycle",
			// x -> a -> b -> c -> d -> e -> c, beside an acyclic chain y -> z, w -> z
			proxy: []map[string]any{
				{"name": "x", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "a"},
				{"name": "a", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "b"},
				{"name": "b", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "c"},
				{"name": "c", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "d"},
				{"name": "d", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "e"},
				{"name": "e", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "c"},
				{"name": "y", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "z"},
				{"name": "z", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "w", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "z"},
			},
			errs: []string{
				"proxy [x] has circular dialer-proxy dependency",
				"proxy [a] has circular dialer-proxy dependency",
				"proxy [b] has circular dialer-proxy dependency",
				"proxy [c] has circular dialer-proxy dependency",
				"proxy [d] has circular dialer-proxy dependency",
				"proxy [e] has circular dialer-proxy dependency",
			},
		},
		{
			testName: "SharedRelayBesideCycle",
			// p and q share an acyclic relay, t leads into the cycle r <-> s
			proxy: []map[string]any{
				{"name": "p", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "relay"},
				{"name": "q", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "relay"},
				{"name": "relay", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "r", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "s"},
				{"name": "s", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "r"},
				{"name": "t", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "s"},
			},
			errs: []string{
				"proxy [r] has circular dialer-proxy dependency",
				"proxy [s] has circular dialer-proxy dependency",
				"proxy [t] has circular dialer-proxy dependency",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			config := RawConfig{Proxy: testCase.proxy}
			// repeat so that different map iteration orders are covered
			for i := 0; i < 20; i++ {
				_, _, err := parseProxies(&config)
				if testCase.errs == nil {
					assert.NoError(t, err)
				} else if assert.Error(t, err) {
					assert.Contains(t, testCase.errs, err.Error())
				}
			}
		})
	}
}

func TestValidateProviderDialerProxies(t *testing.T) {
	providerPayload := func(dialer string) []map[string]any {
		return []map[string]any{
			{"name": "source", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": dialer},
			{"name": "hidden", "type": "socks5", "server": "127.0.0.1", "port": 1081},
		}
	}
	tests := []struct {
		name        string
		proxy       []map[string]any
		groups      []map[string]any
		providers   map[string]map[string]any
		errContains string
	}{
		{
			name:  "top level cannot reference provider-only proxy",
			proxy: []map[string]any{{"name": "top", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "hidden"}},
			providers: map[string]map[string]any{
				"provider-a": {"type": "inline", "payload": providerPayload("")},
			},
			errContains: "not found",
		},
		{
			name:  "provider can reference top-level static proxy",
			proxy: []map[string]any{{"name": "top", "type": "socks5", "server": "127.0.0.1", "port": 1080}},
			providers: map[string]map[string]any{
				"provider-a": {"type": "inline", "filter": "^source$", "payload": providerPayload("top")},
			},
		},
		{
			name:   "provider can reference top-level group",
			groups: []map[string]any{{"name": "top-group", "type": "select", "proxies": []string{"DIRECT"}}},
			providers: map[string]map[string]any{
				"provider-a": {"type": "inline", "filter": "^source$", "payload": providerPayload("top-group")},
			},
		},
		{
			name: "provider can reference own filtered raw proxy",
			providers: map[string]map[string]any{
				"provider-a": {"type": "inline", "filter": "^source$", "payload": providerPayload("hidden")},
			},
		},
		{
			name: "provider cannot reference another provider raw proxy",
			providers: map[string]map[string]any{
				"provider-a": {"type": "inline", "filter": "^source$", "payload": providerPayload("other-hidden")},
				"provider-b": {"type": "inline", "payload": []map[string]any{{"name": "other-hidden", "type": "socks5", "server": "127.0.0.1", "port": 1082}}},
			},
			errContains: "not found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseProxies(&RawConfig{Proxy: test.proxy, ProxyGroup: test.groups, ProxyProvider: test.providers})
			if test.errContains == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, test.errContains)
			}
		})
	}
}
