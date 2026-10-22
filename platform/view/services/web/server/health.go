/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"encoding/json"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

// Healthz reports liveness: it responds 200 to every request and never looks at external
// components, so an outage of a peer or orderer does not make every node fail liveness at once.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Readyz returns a handler that reports readiness from the connection states tracked by m.
// It responds 503 when a connection is stopped, or when a component (network, channel, and
// component labels) has not been connected through any of its endpoints since startup;
// otherwise 200. Reconnecting does not make the node unready. The body is the JSON list of
// the connections that make the node unready.
func Readyz(m *connstate.Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		notReady := notReady(m.Snapshot())
		w.Header().Set("Content-Type", "application/json")
		if len(notReady) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(notReady); err != nil {
			logger.Warnf("failed writing readiness response: %v", err)
		}
	})
}

// notReady returns the statuses that make the node unready. Connected-ness is judged per
// component rather than per endpoint, because delivery moves between peers and a peer it
// moves to has not been connected yet.
func notReady(statuses []connstate.Status) []connstate.Status {
	type component struct{ network, channel, name string }
	connected := map[component]bool{}
	for _, s := range statuses {
		if s.EverConnected {
			connected[component{s.Network, s.Channel, s.Component}] = true
		}
	}
	out := []connstate.Status{}
	for _, s := range statuses {
		if s.State == connstate.Stopped || !connected[component{s.Network, s.Channel, s.Component}] {
			out = append(out, s)
		}
	}
	return out
}
