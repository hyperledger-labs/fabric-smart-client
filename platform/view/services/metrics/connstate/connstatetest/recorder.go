/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package connstatetest records the connection states a component reports through
// connstate, so tests can assert on them without a Prometheus registry.
package connstatetest

import (
	"slices"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// Recorder is a metrics.Provider that records every value set on the
// fsc_connection_state gauge, keyed by the endpoint label. Other metrics are
// discarded. It is safe for concurrent use.
type Recorder struct {
	disabled.Provider

	mu      sync.Mutex
	history map[string][]connstate.State
}

// NewMetrics returns connection-state metrics backed by a new Recorder.
func NewMetrics() (*connstate.Metrics, *Recorder) {
	r := &Recorder{history: map[string][]connstate.State{}}
	return connstate.NewMetrics(r), r
}

// NewGauge returns a recording gauge for fsc_connection_state and a no-op gauge otherwise.
func (r *Recorder) NewGauge(o metrics.GaugeOpts) metrics.Gauge {
	if o.Name != "state" {
		return &disabled.Gauge{}
	}
	return &gauge{r: r}
}

// States returns the states reported for endpoint, oldest first.
func (r *Recorder) States(endpoint string) []connstate.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.history[endpoint])
}

// Last returns the most recent state reported for endpoint, and false if none was.
func (r *Recorder) Last(endpoint string) (connstate.State, bool) {
	states := r.States(endpoint)
	if len(states) == 0 {
		return 0, false
	}
	return states[len(states)-1], true
}

type gauge struct {
	r        *Recorder
	endpoint string
}

func (g *gauge) With(labelValues ...string) metrics.Gauge {
	endpoint := g.endpoint
	for i := 0; i+1 < len(labelValues); i += 2 {
		if labelValues[i] == "endpoint" {
			endpoint = labelValues[i+1]
		}
	}
	return &gauge{r: g.r, endpoint: endpoint}
}

func (*gauge) Add(float64) {}

func (g *gauge) Set(v float64) {
	g.r.mu.Lock()
	defer g.r.mu.Unlock()
	g.r.history[g.endpoint] = append(g.r.history[g.endpoint], connstate.State(v))
}
