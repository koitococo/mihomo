package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDialerProxies(t *testing.T) {
	testCases := []struct {
		testName    string
		proxy       []map[string]any
		errContains string
	}{
		{
			testName: "ValidReference",
			proxy: []map[string]any{ // create proxy with valid dialer-proxy reference
				{"name": "base-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "proxy-with-dialer", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "base-proxy"},
			},
			errContains: "",
		},
		{
			testName: "NotFoundReference",
			proxy: []map[string]any{ // create proxy with non-existent dialer-proxy reference
				{"name": "proxy-with-dialer", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "non-existent-proxy"},
			},
			errContains: "not found",
		},
		{
			testName: "CircularDependency",
			proxy: []map[string]any{
				// create proxy A that references B
				{"name": "proxy-a", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "proxy-c"},
				// create proxy B that references C
				{"name": "proxy-b", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "proxy-a"},
				// create proxy C that references A (creates cycle)
				{"name": "proxy-c", "type": "socks5", "server": "127.0.0.1", "port": 1082, "dialer-proxy": "proxy-a"},
			},
			errContains: "circular",
		},
		{
			testName: "ComplexChain",
			proxy: []map[string]any{ // create a valid chain: proxy-d -> proxy-c -> proxy-b -> proxy-a
				{"name": "proxy-a", "type": "socks5", "server": "127.0.0.1", "port": 1080},
				{"name": "proxy-b", "type": "socks5", "server": "127.0.0.1", "port": 1081, "dialer-proxy": "proxy-a"},
				{"name": "proxy-c", "type": "socks5", "server": "127.0.0.1", "port": 1082, "dialer-proxy": "proxy-b"},
				{"name": "proxy-d", "type": "socks5", "server": "127.0.0.1", "port": 1083, "dialer-proxy": "proxy-c"},
			},
			errContains: "",
		},
		{
			testName: "EmptyDialerProxy",
			proxy: []map[string]any{ // create proxy without dialer-proxy
				{"name": "simple-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080},
			},
			errContains: "",
		},
		{
			testName: "SelfReference",
			proxy: []map[string]any{ // create proxy that references itself
				{"name": "self-proxy", "type": "socks5", "server": "127.0.0.1", "port": 1080, "dialer-proxy": "self-proxy"},
			},
			errContains: "circular",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			config := RawConfig{Proxy: testCase.proxy}
			_, _, err := parseProxies(&config)
			if testCase.errContains == "" {
				assert.NoError(t, err, testCase.testName)
			} else {
				assert.ErrorContains(t, err, testCase.errContains, testCase.testName)
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
