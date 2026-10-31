/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

func readyz(t *testing.T, m *connstate.Metrics) (int, []connstate.Status) {
	t.Helper()
	rec := httptest.NewRecorder()
	server.Readyz(m).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.NotContains(t, rec.Body.String(), "7051")
	var body []connstate.Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec.Code, body
}

func TestReadyz_NotReadyBeforeFirstConnect(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Connecting)

	code, body := readyz(t, m)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, []connstate.Status{{Network: "net", Channel: "ch", Component: connstate.ComponentDelivery, Reason: connstate.ReasonNeverConnected}}, body)
}

func TestReadyz_ReadyWhileReconnecting(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	tr := m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051")
	tr.Set(connstate.Connected)
	tr.Set(connstate.Reconnecting)

	code, body := readyz(t, m)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, body)
}

func TestReadyz_ReadyAfterPeerSwitch(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Connected)
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Disconnected)
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer1:7051").Set(connstate.Reconnecting)

	code, _ := readyz(t, m)
	assert.Equal(t, http.StatusOK, code)
}

func TestReadyz_NotReadyWhenStopped(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "", connstate.ComponentOrderer, "orderer:7050").Set(connstate.Connected)
	tr := m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051")
	tr.Set(connstate.Connected)
	tr.Set(connstate.Stopped)

	code, body := readyz(t, m)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, []connstate.Status{{Network: "net", Channel: "ch", Component: connstate.ComponentDelivery, Reason: connstate.ReasonStopped}}, body)
}

func TestReadyz_OneEntryPerComponent(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Stopped)
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer1:7051").Set(connstate.Connecting)

	code, body := readyz(t, m)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, []connstate.Status{{Network: "net", Channel: "ch", Component: connstate.ComponentDelivery, Reason: connstate.ReasonStopped}}, body)
}

func TestHealthz_IgnoresComponents(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Stopped)

	rec := httptest.NewRecorder()
	server.Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

type gate struct {
	reason   string
	notReady bool
}

func (g gate) NotReady() (string, bool) { return g.reason, g.notReady }

func TestReadyz_GateNotReady(t *testing.T) {
	t.Parallel()
	// A gate that is not ready wins over connections that are.
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Connected)

	rec := httptest.NewRecorder()
	server.Readyz(m, gate{}, gate{reason: "draining", notReady: true}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"not ready","reason":"draining"}`, rec.Body.String())
}

func TestReadyz_GatesReadyKeepsConnstate(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Connecting)

	rec := httptest.NewRecorder()
	server.Readyz(m, gate{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	var body []connstate.Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, connstate.ReasonNeverConnected, body[0].Reason)
}
