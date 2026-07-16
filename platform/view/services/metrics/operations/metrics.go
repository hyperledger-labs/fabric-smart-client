/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

var fscVersion = metrics.GaugeOpts{
	Name:       "fsc_version",
	Help:       "The active version of Fabric Smart Client.",
	LabelNames: []string{"version"},
}

func versionGauge(provider metrics.Provider) metrics.Gauge {
	return provider.NewGauge(fscVersion)
}

func NewMetricsProvider(m MetricsOptions) metrics.Provider {
	switch m.Provider {
	case "prometheus":
		return &prometheus.Provider{}
	default:
		return &disabled.Provider{}
	}
}

type disabledHistogramsProvider struct {
	metrics.Provider
	disabledProvider *disabled.Provider
}

func NewDisabledHistogram(provider metrics.Provider) *disabledHistogramsProvider {
	return &disabledHistogramsProvider{
		Provider:         provider,
		disabledProvider: &disabled.Provider{},
	}
}

func (p *disabledHistogramsProvider) NewHistogram(metrics.HistogramOpts) metrics.Histogram {
	return p.disabledProvider.NewHistogram(metrics.HistogramOpts{})
}
