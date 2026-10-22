/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package integration

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsi/gomega"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

// scrapes returns a metrics source that serves pages in turn and then keeps
// serving the last one, as a node whose state settles would.
func scrapes(t *testing.T, pages ...string) func() (map[string]*dto.MetricFamily, error) {
	t.Helper()
	var n atomic.Int32
	return func() (map[string]*dto.MetricFamily, error) {
		i := min(int(n.Add(1))-1, len(pages)-1)
		parser := expfmt.NewTextParser(model.LegacyValidation)
		return parser.TextToMetricFamilies(strings.NewReader("# TYPE fsc_connection_state gauge\n" + pages[i]))
	}
}

func series(network, endpoint string, s connstate.State) string {
	return `fsc_connection_state{channel="",component="orderer",endpoint="` + endpoint + `",network="` + network + `"} ` + strconv.Itoa(int(s)) + "\n"
}

// failures runs eventuallyConnectionState and returns its failure messages.
func failures(t *testing.T, metrics func() (map[string]*dto.MetricFamily, error), conn fsc.Connection, want connstate.State) []string {
	t.Helper()
	var msgs []string
	g := gomega.NewGomega(func(message string, _ ...int) { msgs = append(msgs, message) })
	eventuallyConnectionState(g, metrics, conn, want, 3*time.Second)
	return msgs
}

func TestEventuallyConnectionState(t *testing.T) {
	t.Parallel()
	orderers := fsc.Connection{Component: connstate.ComponentOrderer}

	t.Run("waits until the state is reached", func(t *testing.T) {
		t.Parallel()
		metrics := scrapes(t,
			series("n1", "e1", connstate.Reconnecting),
			series("n1", "e1", connstate.Reconnecting),
			series("n1", "e1", connstate.Connected))
		require.Empty(t, failures(t, metrics, orderers, connstate.Connected))
	})

	t.Run("fails when the state is never reached", func(t *testing.T) {
		t.Parallel()
		metrics := scrapes(t, series("n1", "e1", connstate.Reconnecting))
		require.NotEmpty(t, failures(t, metrics, orderers, connstate.Connected))
	})

	t.Run("fails when nothing matches", func(t *testing.T) {
		t.Parallel()
		metrics := scrapes(t, series("n1", "e1", connstate.Connected))
		require.NotEmpty(t, failures(t, metrics, fsc.Connection{Endpoint: "e2"}, connstate.Connected))
	})

	t.Run("one network down fails an unscoped check", func(t *testing.T) {
		t.Parallel()
		metrics := scrapes(t, series("n1", "e1", connstate.Connected)+series("n2", "e1", connstate.Disconnected))
		require.NotEmpty(t, failures(t, metrics, fsc.Connection{Endpoint: "e1"}, connstate.Connected),
			"n2 is down on the shared endpoint, so the check must not pass")
		require.Empty(t, failures(t, metrics, fsc.Connection{Network: "n1", Endpoint: "e1"}, connstate.Connected))
	})
}
