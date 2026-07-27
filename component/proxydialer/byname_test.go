package proxydialer

import (
	"testing"

	"github.com/stretchr/testify/require"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type byNameTestProxy struct{ C.Proxy }

type byNameTestProvider struct {
	P.ProxyProvider
	targets map[string]C.Proxy
}

func (p *byNameTestProvider) GetDialerProxy(name string) (C.Proxy, bool) {
	proxy, ok := p.targets[name]
	return proxy, ok
}

type byNameTestTunnel struct {
	C.Tunnel
	proxies   map[string]C.Proxy
	providers map[string]P.ProxyProvider
}

func (t *byNameTestTunnel) Proxies() map[string]C.Proxy           { return t.proxies }
func (t *byNameTestTunnel) Providers() map[string]P.ProxyProvider { return t.providers }

func TestByNameProxyDialerProxy(t *testing.T) {
	topLevel := &byNameTestProxy{}
	providerTarget := &byNameTestProxy{}
	replacement := &byNameTestProxy{}
	provider := &byNameTestProvider{targets: map[string]C.Proxy{"shared": providerTarget, "hidden": providerTarget}}
	tunnel := &byNameTestTunnel{
		proxies:   map[string]C.Proxy{"shared": topLevel},
		providers: map[string]P.ProxyProvider{"provider": provider},
	}

	t.Run("top level wins", func(t *testing.T) {
		proxy, err := byNameProxyDialer{proxyName: "shared", providerName: "provider", tunnel: tunnel}.proxy()
		require.NoError(t, err)
		require.Same(t, topLevel, proxy)
	})
	t.Run("falls back to owning provider", func(t *testing.T) {
		proxy, err := byNameProxyDialer{proxyName: "hidden", providerName: "provider", tunnel: tunnel}.proxy()
		require.NoError(t, err)
		require.Same(t, providerTarget, proxy)
	})
	t.Run("missing targets preserve error", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			provider string
			target   string
		}{
			{name: "empty provider", target: "hidden"},
			{name: "unknown provider", provider: "unknown", target: "hidden"},
			{name: "missing target", provider: "provider", target: "missing"},
		} {
			t.Run(test.name, func(t *testing.T) {
				_, err := byNameProxyDialer{proxyName: test.target, providerName: test.provider, tunnel: tunnel}.proxy()
				require.EqualError(t, err, "proxyName["+test.target+"] not found")
			})
		}
	})
	t.Run("observes refreshed provider target", func(t *testing.T) {
		dialer := byNameProxyDialer{proxyName: "hidden", providerName: "provider", tunnel: tunnel}
		proxy, err := dialer.proxy()
		require.NoError(t, err)
		require.Same(t, providerTarget, proxy)
		provider.targets = map[string]C.Proxy{"hidden": replacement}
		proxy, err = dialer.proxy()
		require.NoError(t, err)
		require.Same(t, replacement, proxy)
	})
}
