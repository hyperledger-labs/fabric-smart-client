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

// ReadinessGate reports a reason the node is not ready that is not a connection, such as
// draining before shutdown.
type ReadinessGate interface {
	// NotReady returns the reason and true while the node is not ready.
	NotReady() (reason string, notReady bool)
}

// Readyz returns a handler that reports readiness. A gate that is not ready takes precedence:
// the handler responds 503 with the body {"status":"not ready","reason":"<reason>"} of the first
// such gate. Otherwise it reports from the connection states tracked by m: it responds 503 when
// a connection is stopped, or when a component (network, channel, and component labels) has not
// been connected through any of its endpoints since startup; otherwise 200. Reconnecting does
// not make the node unready. The body is then the JSON list of the unready components with their
// reason.
func Readyz(m *connstate.Metrics, gates ...ReadinessGate) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body any
		for _, g := range gates {
			if reason, notReady := g.NotReady(); notReady {
				w.WriteHeader(http.StatusServiceUnavailable)
				body = map[string]string{"status": "not ready", "reason": reason}
				break
			}
		}
		if body == nil {
			notReady := notReady(m.Snapshot())
			if len(notReady) > 0 {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			body = notReady
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			logger.Warnf("failed writing readiness response: %v", err)
		}
	})
}

// notReady returns the unready components, once each and in the order of statuses.
// Connected-ness is judged per component rather than per endpoint, because delivery moves
// between peers and a peer it moves to has not been connected yet. A component with a stopped
// connection is reported as stopped, since it needs operator action.
func notReady(statuses []connstate.Status) []connstate.Status {
	type component struct{ network, channel, name string }
	var order []component
	reasons := map[component]string{}
	for _, s := range statuses {
		c := component{s.Network, s.Channel, s.Component}
		prev, seen := reasons[c]
		if !seen {
			order = append(order, c)
		}
		// Stopped overrides everything; a connection that has connected overrides never connected.
		if !seen || prev == connstate.ReasonNeverConnected || s.Reason == connstate.ReasonStopped {
			reasons[c] = s.Reason
		}
	}
	out := []connstate.Status{}
	for _, c := range order {
		if r := reasons[c]; r != "" {
			out = append(out, connstate.Status{Network: c.network, Channel: c.channel, Component: c.name, Reason: r})
		}
	}
	return out
}
