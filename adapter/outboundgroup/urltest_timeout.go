package outboundgroup

import (
	"context"
	"time"
)

type urlTestTimeoutKey struct{}

// WithURLTestTimeout attaches a per-proxy URLTest budget.
// GroupBase.URLTest applies this to each proxy instead of sharing one overall deadline.
func WithURLTestTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, urlTestTimeoutKey{}, timeout)
}

// URLTestTimeoutFromContext returns the per-proxy URLTest budget if present.
func URLTestTimeoutFromContext(ctx context.Context) (time.Duration, bool) {
	timeout, ok := ctx.Value(urlTestTimeoutKey{}).(time.Duration)
	return timeout, ok && timeout > 0
}
