/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"strings"
	"sync"

	log2 "github.com/go-kit/kit/log"
	kitstatsd "github.com/go-kit/kit/metrics/statsd"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/statsd"
)

var (
	fscVersion = metrics.GaugeOpts{
		Name:         "fsc_version",
		Help:         "The active version of Fabric Smart Client.",
		LabelNames:   []string{"version"},
		StatsdFormat: "%{#fqname}.%{version}",
	}

	gaugeLock        sync.Mutex
	promVersionGauge metrics.Gauge
)

func versionGauge(provider metrics.Provider) metrics.Gauge {
	switch provider.(type) {
	case *prometheus.Provider:
		gaugeLock.Lock()
		defer gaugeLock.Unlock()
		if promVersionGauge == nil {
			promVersionGauge = provider.NewGauge(fscVersion)
		}
		return promVersionGauge

	default:
		return provider.NewGauge(fscVersion)
	}
}

func NewMetricsProvider(m MetricsOptions, l log2.Logger) metrics.Provider {
	switch m.Provider {
	case "statsd":
		prefix := m.Statsd.Prefix
		if prefix != "" && !strings.HasSuffix(prefix, ".") {
			prefix = prefix + "."
		}

		ks := kitstatsd.New(prefix, l)
		return &statsd.Provider{Statsd: ks}
	case "prometheus":
		return &prometheus.Provider{}
	default:
		return &disabled.Provider{}
	}
}
