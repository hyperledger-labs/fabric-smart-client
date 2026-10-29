/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package connstate exports the connection state of FSC's long-lived client connections
// (orderer, delivery, Fabric-x notification and query services, database) as the
// fsc_connection_* metric families, all labeled by network, channel, component, and endpoint.
package connstate

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

// State is the value of the fsc_connection_state gauge.
type State int

const (
	// Disconnected means no connection is open; it is also the final state after a clean shutdown.
	Disconnected State = iota
	// Connecting means the first connection attempt is in progress.
	Connecting
	// Connected means the connection is established.
	Connected
	// Reconnecting means a lost connection is being re-established.
	Reconnecting
	// Stopped means the component gave up retrying; recovery requires operator action.
	Stopped
)

// Values of the component label.
const (
	ComponentOrderer             = "orderer"
	ComponentDelivery            = "delivery"
	ComponentFabricxNotification = "fabricx_notification"
	ComponentFabricxQuery        = "fabricx_query"
	ComponentDatabase            = "database"
)

var labelNames = []string{"network", "channel", "component", "endpoint"}

type key struct{ network, channel, component, endpoint string }

// Metrics holds the connection-state metric families and the trackers created from them.
// It is safe for concurrent use.
type Metrics struct {
	state         metrics.Gauge
	reconnects    metrics.Counter
	lastConnected metrics.Gauge

	mu       sync.Mutex
	trackers map[key]*Tracker
}

// NewMetrics registers the fsc_connection_* metric families with p.
func NewMetrics(p metrics.Provider) *Metrics {
	return &Metrics{
		state: p.NewGauge(metrics.GaugeOpts{
			Namespace:  "fsc",
			Subsystem:  "connection",
			Name:       "state",
			Help:       "Connection state: 0=disconnected, 1=connecting, 2=connected, 3=reconnecting, 4=stopped.",
			LabelNames: labelNames,
		}),
		reconnects: p.NewCounter(metrics.CounterOpts{
			Namespace:  "fsc",
			Subsystem:  "connection",
			Name:       "reconnects",
			Help:       "Number of reconnect attempts.",
			LabelNames: labelNames,
		}),
		lastConnected: p.NewGauge(metrics.GaugeOpts{
			Namespace:  "fsc",
			Subsystem:  "connection",
			Name:       "last_connected_timestamp_seconds",
			Help:       "Unix time of the last transition to the connected state.",
			LabelNames: labelNames,
		}),
		trackers: map[key]*Tracker{},
	}
}

// Tracker returns the tracker of one connection; the same arguments return the same *Tracker.
// A nil *Metrics returns a tracker that records nothing.
func (m *Metrics) Tracker(network, channel, component, endpoint string) *Tracker {
	if m == nil {
		return &Tracker{}
	}
	k := key{network, channel, component, endpoint}
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.trackers[k]; ok {
		return t
	}
	labels := []string{"network", network, "channel", channel, "component", component, "endpoint", endpoint}
	t := &Tracker{
		state:         m.state.With(labels...),
		reconnects:    m.reconnects.With(labels...),
		lastConnected: m.lastConnected.With(labels...),
	}
	m.trackers[k] = t
	return t
}

// Values of Status.Reason.
const (
	ReasonStopped        = "stopped"
	ReasonNeverConnected = "never connected"
)

// Status is the state of one tracked connection. It names no endpoint, so it can be served
// without authentication.
type Status struct {
	Network   string `json:"network"`
	Channel   string `json:"channel"`
	Component string `json:"component"`
	// Reason is ReasonStopped, ReasonNeverConnected if the connection has not been Connected
	// since the tracker was created, or empty otherwise.
	Reason string `json:"reason"`
}

// Snapshot returns the status of every tracker created by m, sorted by network, channel, and
// component; connections of the same component are in no particular order. A nil *Metrics
// returns nil.
func (m *Metrics) Snapshot() []Status {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	out := make([]Status, 0, len(m.trackers))
	for k, t := range m.trackers {
		t.mu.Lock()
		s := Status{Network: k.network, Channel: k.channel, Component: k.component}
		switch {
		case t.current == Stopped:
			s.Reason = ReasonStopped
		case !t.everConnected:
			s.Reason = ReasonNeverConnected
		}
		out = append(out, s)
		t.mu.Unlock()
	}
	m.mu.Unlock()
	slices.SortFunc(out, func(a, b Status) int {
		return cmp.Or(cmp.Compare(a.Network, b.Network), cmp.Compare(a.Channel, b.Channel),
			cmp.Compare(a.Component, b.Component))
	})
	return out
}

// Tracker records the state of one connection.
type Tracker struct {
	state         metrics.Gauge
	reconnects    metrics.Counter
	lastConnected metrics.Gauge

	mu            sync.Mutex
	current       State
	everConnected bool
}

// Set records s as the current state. Reconnecting increments the reconnect counter,
// and Connected stamps the last-connected time. A nil *Tracker records nothing.
func (t *Tracker) Set(s State) {
	if t == nil || t.state == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = s
	t.state.Set(float64(s))
	switch s {
	case Reconnecting:
		t.reconnects.Add(1)
	case Connected:
		t.everConnected = true
		t.lastConnected.Set(float64(time.Now().Unix()))
	}
}
