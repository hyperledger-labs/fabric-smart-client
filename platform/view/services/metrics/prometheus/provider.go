/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	kitmetrics "github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/prometheus"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	prom "github.com/prometheus/client_golang/prometheus"
)

type Provider struct {
	SkipRegisterErr bool
}

var (
	replacersMutex sync.RWMutex
	replacers      = map[string]string{
		"github.com_hyperledger-labs_fabric-smart-client_platform": "fsc",
	}
)

func RegisterReplacer(s string, replaceWith string) {
	replacersMutex.Lock()
	defer replacersMutex.Unlock()

	_, ok := replacers[s]
	if ok {
		panic("replacer already exists")
	}

	replacers[s] = replaceWith
}

func Replacers() map[string]string {
	replacersMutex.RLock()
	defer replacersMutex.RUnlock()
	return replacers
}

func (p *Provider) applyNamespaceSubsystem(namespace, subsystem *string) {
	ns, ss := parseFullPkgName(GetPackageName(), Replacers())
	if len(*namespace) == 0 {
		*namespace = ns
	}
	if len(*subsystem) == 0 {
		*subsystem = ss
	}
}

func (p *Provider) NewCounter(o metrics.CounterOpts) metrics.Counter {
	p.applyNamespaceSubsystem(&o.Namespace, &o.Subsystem)

	cv := prom.NewCounterVec(prom.CounterOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
	}, o.LabelNames)
	p.register(cv)
	return &Counter{Counter: prometheus.NewCounter(cv)}
}

func (p *Provider) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	p.applyNamespaceSubsystem(&o.Namespace, &o.Subsystem)

	gv := prom.NewGaugeVec(prom.GaugeOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
	}, o.LabelNames)
	p.register(gv)
	return &Gauge{Gauge: prometheus.NewGauge(gv)}
}

func (p *Provider) NewHistogram(o metrics.HistogramOpts) metrics.Histogram {
	p.applyNamespaceSubsystem(&o.Namespace, &o.Subsystem)

	hv := prom.NewHistogramVec(prom.HistogramOpts{
		Namespace: o.Namespace,
		Subsystem: o.Subsystem,
		Name:      o.Name,
		Help:      o.Help,
		Buckets:   o.Buckets,
	}, o.LabelNames)
	p.register(hv)
	return &Histogram{prometheus.NewHistogram(hv)}
}

func GetPackageName() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("GetPackageName: unable to retrieve caller information using runtime.Caller")
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		panic(fmt.Sprintf("GetPackageName: unable to retrieve function for PC: %v", pc))
	}
	fullFuncName := fn.Name()
	lastSlash := strings.LastIndex(fullFuncName, "/")
	dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
	return fullFuncName[:lastSlash+dotAfterSlash]
}

func parseFullPkgName(fullPkgName string, replacements map[string]string, params ...string) (string, string) {
	parts := append(strings.Split(fullPkgName, "/"), params...)
	subsystem := parts[len(parts)-1]
	namespaceParts := parts[:len(parts)-1]
	namespace := strings.Join(namespaceParts, "_")

	for old, newVal := range replacements {
		namespace = strings.ReplaceAll(namespace, old, newVal)
	}
	return namespace, subsystem
}

func (p *Provider) register(c prom.Collector) {
	if err := prom.Register(c); err != nil && !p.SkipRegisterErr {
		panic(err)
	}
}

type Counter struct{ kitmetrics.Counter }

func (c *Counter) With(labelValues ...string) metrics.Counter {
	return &Counter{Counter: c.Counter.With(labelValues...)}
}

type Gauge struct{ kitmetrics.Gauge }

func (g *Gauge) With(labelValues ...string) metrics.Gauge {
	return &Gauge{Gauge: g.Gauge.With(labelValues...)}
}

type Histogram struct{ kitmetrics.Histogram }

func (h *Histogram) With(labelValues ...string) metrics.Histogram {
	return &Histogram{Histogram: h.Histogram.With(labelValues...)}
}
