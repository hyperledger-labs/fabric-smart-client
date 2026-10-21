/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package connstate

import (
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
	for _, s := range []State{Disconnected, Connecting, Connected, Reconnecting, Stopped} {
		tr.Set(s)
	}
}
