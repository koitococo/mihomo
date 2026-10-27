package provider

import (
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

type automaticSkipTestProxy struct {
	C.Proxy
	probability float64
}

func (p automaticSkipTestProxy) AutomaticURLTestSkipProbability() float64 {
	return p.probability
}

func TestShouldSkipAutomaticURLTest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		proxy  C.Proxy
		sample float64
		want   bool
	}{
		{"unsupported", struct{ C.Proxy }{}, 0, false},
		{"ineligible", automaticSkipTestProxy{probability: 0}, 0, false},
		{"below minimum", automaticSkipTestProxy{probability: 0.05}, 0.049, true},
		{"at minimum", automaticSkipTestProxy{probability: 0.05}, 0.05, false},
		{"below cap", automaticSkipTestProxy{probability: 0.95}, 0.949, true},
		{"at cap", automaticSkipTestProxy{probability: 0.95}, 0.95, false},
		{"retry above cap", automaticSkipTestProxy{probability: 0.95}, 0.999, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSkipAutomaticURLTest(tc.proxy, tc.sample); got != tc.want {
				t.Fatalf("shouldSkipAutomaticURLTest() = %t, want %t", got, tc.want)
			}
		})
	}
}
