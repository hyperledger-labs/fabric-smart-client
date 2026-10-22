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
	require.Len(t, body, 1)
	assert.Equal(t, "peer0:7051", body[0].Endpoint)
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
	require.Len(t, body, 1)
	assert.Equal(t, connstate.ComponentDelivery, body[0].Component)
	assert.Equal(t, connstate.Stopped, body[0].State)
}

func TestHealthz_IgnoresComponents(t *testing.T) {
	t.Parallel()
	m := connstate.NewMetrics(&disabled.Provider{})
	m.Tracker("net", "ch", connstate.ComponentDelivery, "peer0:7051").Set(connstate.Stopped)

	rec := httptest.NewRecorder()
	server.Healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}
