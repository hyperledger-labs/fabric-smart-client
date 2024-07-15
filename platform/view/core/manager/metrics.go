package manager

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"

type Metrics struct {
	Contexts metrics.Gauge
}

func newMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		Contexts: p.NewGauge(metrics.GaugeOpts{
			Namespace:    "host",
			Name:         "host_contexts",
			Help:         "The number of open contexts on the client side",
			StatsdFormat: "%{#fqname}",
		}),
	}
}
