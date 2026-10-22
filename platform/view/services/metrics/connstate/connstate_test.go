/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package connstate

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

func newTestMetrics() (*Metrics, *mock.Gauge, *mock.Counter, *mock.Gauge) {
	state, reconnects, lastConnected := &mock.Gauge{}, &mock.Counter{}, &mock.Gauge{}
	state.WithReturns(state)
	reconnects.WithReturns(reconnects)
	lastConnected.WithReturns(lastConnected)
	p := &mock.Provider{}
	p.NewGaugeCalls(func(o metrics.GaugeOpts) metrics.Gauge {
		if o.Name == "state" {
			return state
		}
		return lastConnected
	})
	p.NewCounterReturns(reconnects)
	return NewMetrics(p), state, reconnects, lastConnected
}

func TestTracker_SameKeySameTracker(t *testing.T) {
	t.Parallel()
	m, state, _, _ := newTestMetrics()
	a := m.Tracker("net", "ch", ComponentDelivery, "peer0:7051")
	assert.Same(t, a, m.Tracker("net", "ch", ComponentDelivery, "peer0:7051"))
	assert.NotSame(t, a, m.Tracker("net", "ch", ComponentDelivery, "peer1:7051"))
	require.Equal(t, 2, state.WithCallCount())
	assert.Equal(t, []string{"network", "net", "channel", "ch", "component", "delivery", "endpoint", "peer0:7051"}, state.WithArgsForCall(0))
}

func TestTracker_ReconnectingIncrementsCounter(t *testing.T) {
	t.Parallel()
	m, state, reconnects, _ := newTestMetrics()
	tr := m.Tracker("net", "ch", ComponentOrderer, "orderer:7050")
	tr.Set(Reconnecting)
	tr.Set(Reconnecting)
	require.Equal(t, 2, reconnects.AddCallCount())
	assert.InDelta(t, 2.0, reconnects.AddArgsForCall(0)+reconnects.AddArgsForCall(1), 0)
	assert.InDelta(t, float64(Reconnecting), state.SetArgsForCall(1), 0)
}

func TestTracker_ConnectedStampsTime(t *testing.T) {
	t.Parallel()
	m, state, reconnects, lastConnected := newTestMetrics()
	m.Tracker("net", "", ComponentDatabase, "db").Set(Connected)
	assert.InDelta(t, float64(Connected), state.SetArgsForCall(0), 0)
	assert.Equal(t, 0, reconnects.AddCallCount())
	require.Equal(t, 1, lastConnected.SetCallCount())
	assert.InDelta(t, float64(time.Now().Unix()), lastConnected.SetArgsForCall(0), 1)
}

func TestNilMetrics_NoOpTracker(t *testing.T) {
	t.Parallel()
	var m *Metrics
	tr := m.Tracker("net", "ch", ComponentFabricxQuery, "q:7001")
	require.NotNil(t, tr)
	var nilTracker *Tracker
	for _, s := range []State{Disconnected, Connecting, Connected, Reconnecting, Stopped} {
		tr.Set(s)
		nilTracker.Set(s)
	}
}

func TestSnapshot_ReportsEverConnected(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newTestMetrics()
	m.Tracker("net", "ch", ComponentDelivery, "peer1:7051").Set(Connecting)
	tr := m.Tracker("net", "ch", ComponentDelivery, "peer0:7051")
	tr.Set(Connected)
	tr.Set(Reconnecting)

	assert.Equal(t, []Status{
		{Network: "net", Channel: "ch", Component: ComponentDelivery, Endpoint: "peer0:7051", State: Reconnecting, EverConnected: true},
		{Network: "net", Channel: "ch", Component: ComponentDelivery, Endpoint: "peer1:7051", State: Connecting},
	}, m.Snapshot())
	var nilMetrics *Metrics
	assert.Nil(t, nilMetrics.Snapshot())
}

func TestSnapshot_ConcurrentSet(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newTestMetrics()
	tr := m.Tracker("net", "ch", ComponentOrderer, "orderer:7050")
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			tr.Set(Connected)
		}
	})
	for range 100 {
		m.Snapshot()
	}
	wg.Wait()
	assert.True(t, m.Snapshot()[0].EverConnected)
}
