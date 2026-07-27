package proxydialer

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

type Tunnel interface {
	C.Tunnel
	Proxies() map[string]C.Proxy
	Providers() map[string]P.ProxyProvider
}

type dialerProxyProvider interface {
	GetDialerProxy(name string) (C.Proxy, bool)
}

type byNameProxyDialer struct {
	proxyName    string
	providerName string
	tunnel       C.Tunnel
}

func (d byNameProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	proxy, err := d.proxy()
	if err != nil {
		return nil, err
	}
	return New(proxy, true).DialContext(ctx, network, address)
}

func (d byNameProxyDialer) proxy() (C.Proxy, error) {
	tunnel, _ := d.tunnel.(Tunnel)
	if tunnel == nil {
		return nil, fmt.Errorf("tunnel is invalid, must be proxydialer.Tunnel, but got: %T", d.tunnel)
	}
	if proxy, ok := tunnel.Proxies()[d.proxyName]; ok {
		return proxy, nil
	}
	if d.providerName != "" {
		if provider, ok := tunnel.Providers()[d.providerName]; ok {
			if dialerProvider, ok := provider.(dialerProxyProvider); ok {
				if proxy, ok := dialerProvider.GetDialerProxy(d.proxyName); ok {
					return proxy, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("proxyName[%s] not found", d.proxyName)
}

func (d byNameProxyDialer) ListenPacket(ctx context.Context, network, address string, rAddrPort netip.AddrPort) (net.PacketConn, error) {
	proxy, err := d.proxy()
	if err != nil {
		return nil, err
	}
	return New(proxy, true).ListenPacket(ctx, network, address, rAddrPort)
}

func NewByName(proxyName, providerName string, tunnel C.Tunnel) C.Dialer {
	return byNameProxyDialer{proxyName: proxyName, providerName: providerName, tunnel: tunnel}
}
