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

// sample is fsc_connection_state as a node exports it: two orderer endpoints
// in different states and one delivery endpoint, plus an unrelated family.
const sample = `# HELP fsc_connection_state Connection state: 0=disconnected, 1=connecting, 2=connected, 3=reconnecting, 4=stopped.
# TYPE fsc_connection_state gauge
fsc_connection_state{channel="",component="orderer",endpoint="127.0.0.1:7050",network="default"} 2
fsc_connection_state{channel="",component="orderer",endpoint="127.0.0.1:7051",network="default"} 3
fsc_connection_state{channel="testchannel",component="delivery",endpoint="127.0.0.1:7060",network="default"} 2
# HELP fsc_connection_reconnects Number of reconnect attempts.
# TYPE fsc_connection_reconnects counter
fsc_connection_reconnects{channel="",component="orderer",endpoint="127.0.0.1:7051",network="default"} 1
`

func TestParseConnectionState(t *testing.T) {
	t.Parallel()
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(sample))
	require.NoError(t, err)

	require.Equal(t, map[string]connstate.State{
		"127.0.0.1:7050": connstate.Connected,
		"127.0.0.1:7051": connstate.Reconnecting,
	}, ParseConnectionState(families, connstate.ComponentOrderer))
	require.Equal(t, map[string]connstate.State{
		"127.0.0.1:7060": connstate.Connected,
	}, ParseConnectionState(families, connstate.ComponentDelivery))
	require.Empty(t, ParseConnectionState(families, connstate.ComponentDatabase))
	require.Empty(t, ParseConnectionState(nil, connstate.ComponentOrderer), "a node without the family reports nothing")
}
