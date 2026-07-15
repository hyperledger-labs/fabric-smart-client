/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package prometheus

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"

	prom "github.com/prometheus/client_golang/prometheus"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

type Provider struct {
	SkipRegisterErr bool

	// cache maps "<kind>|<fully-qualified name>" to the collector handed out
	// for that name, so repeated requests bypass the registry. It assumes the
	// default registerer is not swapped while the provider is in use.
	mu    sync.Mutex
	cache sync.Map
}

type cacheEntry struct {
	opts      any
	collector prom.Collector
}

const (
	// callerSkipFrames represents the number of stack frames to skip when determining
	// the calling package name. This skips: (1) GetPackageName itself, (2) the method
	// calling GetPackageName (e.g., NewCounter/NewGauge/NewHistogram), and (3) the
	// applyNamespaceSubsystem method, to reach the actual caller that initiated the
	// metrics creation.
	callerSkipFrames = 3
)

var (
	replacersMutex sync.RWMutex
	replacers      = map[string]string{
		"github.com_hyperledger-labs_fabric-smart-client_platform": "fsc",
	}
)

func RegisterReplacer(s, replaceWith string) {
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
	// snapshot the retained slices/maps so later caller mutations cannot
	// corrupt the cached definition
	o.LabelNames = slices.Clone(o.LabelNames)
	o.LabelHelp = maps.Clone(o.LabelHelp)

	cv := getOrCreate(p, "counter", prom.BuildFQName(o.Namespace, o.Subsystem, o.Name), o, func() *prom.CounterVec {
		return prom.NewCounterVec(prom.CounterOpts{
			Namespace: o.Namespace,
			Subsystem: o.Subsystem,
			Name:      o.Name,
			Help:      o.Help,
		}, o.LabelNames)
	})
	return &counter{cv: cv}
}

func (p *Provider) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	p.applyNamespaceSubsystem(&o.Namespace, &o.Subsystem)
	o.LabelNames = slices.Clone(o.LabelNames)
	o.LabelHelp = maps.Clone(o.LabelHelp)

	gv := getOrCreate(p, "gauge", prom.BuildFQName(o.Namespace, o.Subsystem, o.Name), o, func() *prom.GaugeVec {
		return prom.NewGaugeVec(prom.GaugeOpts{
			Namespace: o.Namespace,
			Subsystem: o.Subsystem,
			Name:      o.Name,
			Help:      o.Help,
		}, o.LabelNames)
	})
	return &gauge{gv: gv}
}

func (p *Provider) NewHistogram(o metrics.HistogramOpts) metrics.Histogram {
	p.applyNamespaceSubsystem(&o.Namespace, &o.Subsystem)
	o.LabelNames = slices.Clone(o.LabelNames)
	o.LabelHelp = maps.Clone(o.LabelHelp)
	o.Buckets = slices.Clone(o.Buckets)

	hv := getOrCreate(p, "histogram", prom.BuildFQName(o.Namespace, o.Subsystem, o.Name), o, func() *prom.HistogramVec {
		return prom.NewHistogramVec(prom.HistogramOpts{
			Namespace:                      o.Namespace,
			Subsystem:                      o.Subsystem,
			Name:                           o.Name,
			Help:                           o.Help,
			Buckets:                        o.Buckets,
			NativeHistogramBucketFactor:    o.NativeHistogramBucketFactor,
			NativeHistogramMaxBucketNumber: o.NativeHistogramMaxBucketNumber,
			NativeHistogramZeroThreshold:   o.NativeHistogramZeroThreshold,
		}, o.LabelNames)
	})
	return &histogram{hv: hv}
}

func GetPackageName() string {
	pc, _, _, ok := runtime.Caller(callerSkipFrames)
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

// getOrCreate returns the collector cached under kind and fqName, building and
// registering it on the first request so later requests bypass the registry.
func getOrCreate[V prom.Collector, O any](p *Provider, kind, fqName string, opts O, build func() V) V {
	key := kind + "|" + fqName
	e, ok := p.cache.Load(key)
	if !ok {
		// deferred unlock so that a panicking registration does not leave the
		// lock held
		e = func() any {
			p.mu.Lock()
			defer p.mu.Unlock()
			if e, ok := p.cache.Load(key); ok {
				return e
			}
			entry := &cacheEntry{opts: opts, collector: register(p, fqName, build())}
			p.cache.Store(key, entry)
			return entry
		}()
	}
	return fromCache(p, key, e.(*cacheEntry), opts, build)
}

// fromCache returns the cached collector after checking that opts match the
// ones it was created with. On mismatch it panics, or falls back to a fresh
// unregistered collector when SkipRegisterErr is set.
func fromCache[V prom.Collector, O any](p *Provider, key string, e *cacheEntry, opts O, build func() V) V {
	c, sameType := e.collector.(V)
	if sameType && reflect.DeepEqual(e.opts, opts) {
		return c
	}
	if p.SkipRegisterErr {
		return build()
	}
	panic(fmt.Errorf("metric [%s] was requested with a different type or options than it was created with", key))
}

// register registers c and returns it. If an identical collector is already
// registered, that collector is returned instead so all requesters share it.
func register[V prom.Collector](p *Provider, fqName string, c V) prom.Collector {
	err := prom.Register(c)
	if err == nil {
		return c
	}
	are := prom.AlreadyRegisteredError{}
	if errors.As(err, &are) {
		if existing, ok := are.ExistingCollector.(V); ok {
			return existing
		}
		err = fmt.Errorf("metric [%s] is already registered with an incompatible collector type %T", fqName, are.ExistingCollector)
	}
	if p.SkipRegisterErr {
		return c
	}
	panic(err)
}

type counter struct {
	cv  *prom.CounterVec
	lvs labelValues
}

func (c *counter) With(labelValues ...string) metrics.Counter {
	return &counter{
		cv:  c.cv,
		lvs: c.lvs.With(labelValues...),
	}
}

func (c *counter) Add(delta float64) {
	c.cv.With(makeLabels(c.lvs...)).Add(delta)
}

type gauge struct {
	gv  *prom.GaugeVec
	lvs labelValues
}

func (g *gauge) With(labelValues ...string) metrics.Gauge {
	return &gauge{
		gv:  g.gv,
		lvs: g.lvs.With(labelValues...),
	}
}

func (g *gauge) Set(value float64) {
	g.gv.With(makeLabels(g.lvs...)).Set(value)
}

func (g *gauge) Add(delta float64) {
	g.gv.With(makeLabels(g.lvs...)).Add(delta)
}

type histogram struct {
	hv  *prom.HistogramVec
	lvs labelValues
}

func (h *histogram) With(labelValues ...string) metrics.Histogram {
	return &histogram{
		hv:  h.hv,
		lvs: h.lvs.With(labelValues...),
	}
}

func (h *histogram) Observe(value float64) {
	h.hv.With(makeLabels(h.lvs...)).Observe(value)
}

func makeLabels(labelValues ...string) prom.Labels {
	labels := prom.Labels{}
	for i := 0; i < len(labelValues); i += 2 {
		labels[labelValues[i]] = labelValues[i+1]
	}
	return labels
}

type labelValues []string

func (lvs labelValues) With(pairs ...string) labelValues {
	if len(pairs)%2 != 0 {
		pairs = append(pairs, "unknown")
	}
	next := make(labelValues, len(lvs)+len(pairs))
	copy(next, lvs)
	copy(next[len(lvs):], pairs)
	return next
}
