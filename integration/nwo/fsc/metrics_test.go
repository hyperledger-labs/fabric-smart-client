/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fsc

import (
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

// sample is fsc_connection_state as a node exports it: two networks share the
// orderer endpoint 127.0.0.1:7050 in different states, n1 has a second
// orderer, and n1 has a delivery series; plus an unrelated family.
const sample = `# HELP fsc_connection_state Connection state: 0=disconnected, 1=connecting, 2=connected, 3=reconnecting, 4=stopped.
# TYPE fsc_connection_state gauge
fsc_connection_state{channel="",component="orderer",endpoint="127.0.0.1:7050",network="n1"} 2
fsc_connection_state{channel="",component="orderer",endpoint="127.0.0.1:7050",network="n2"} 0
fsc_connection_state{channel="",component="orderer",endpoint="127.0.0.1:7051",network="n1"} 3
fsc_connection_state{channel="testchannel",component="delivery",endpoint="127.0.0.1:7060",network="n1"} 2
# HELP fsc_connection_reconnects Number of reconnect attempts.
# TYPE fsc_connection_reconnects counter
fsc_connection_reconnects{channel="",component="orderer",endpoint="127.0.0.1:7051",network="n1"} 1
`

func TestParseConnectionState(t *testing.T) {
	t.Parallel()
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(sample))
	require.NoError(t, err)

	orderer := func(network, endpoint string) Connection {
		return Connection{Network: network, Component: connstate.ComponentOrderer, Endpoint: endpoint}
	}
	require.Equal(t, map[Connection]connstate.State{
		orderer("n1", "127.0.0.1:7050"): connstate.Connected,
		orderer("n2", "127.0.0.1:7050"): connstate.Disconnected,
		orderer("n1", "127.0.0.1:7051"): connstate.Reconnecting,
	}, ParseConnectionState(families, Connection{Component: connstate.ComponentOrderer}),
		"series of different networks on the same endpoint must not collide")
	require.Equal(t, map[Connection]connstate.State{
		orderer("n2", "127.0.0.1:7050"): connstate.Disconnected,
	}, ParseConnectionState(families, Connection{Network: "n2", Endpoint: "127.0.0.1:7050"}))
	require.Equal(t, map[Connection]connstate.State{
		{Network: "n1", Channel: "testchannel", Component: connstate.ComponentDelivery, Endpoint: "127.0.0.1:7060"}: connstate.Connected,
	}, ParseConnectionState(families, Connection{Component: connstate.ComponentDelivery, Channel: "testchannel"}))
	require.Empty(t, ParseConnectionState(families, Connection{Component: connstate.ComponentDatabase}))
	require.Empty(t, ParseConnectionState(nil, Connection{}), "a node without the family reports nothing")
}
