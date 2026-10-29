/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

// Topology returns the FSC nodes alice, which serves the init view, and bob, which
// answers it under its name and under the alias bob_alias, communicating over commType.
// Both nodes expose Prometheus metrics and export traces to the OTLP collector of the
// monitoring topology.
func Topology(commType fsc.P2PCommunicationType) []api.Topology {
	topology := fsc.NewTopology()
	topology.P2PCommunicationType = commType
	topology.EnableTracing(tracing.Otlp)
	topology.EnablePrometheusMetrics()

	topology.AddNodeByName("alice").
		RegisterViewFactory("init", &InitiatorViewFactory{})

	topology.AddNodeByName("bob").
		RegisterResponder(&Responder{}, &Initiator{}).
		AddOptions(fsc.WithAlias("bob_alias"))

	topology.AddSDKForCommType(&viewsdk.SDK{}, commType)

	monitoringTopology := monitoring.NewTopology()
	monitoringTopology.EnableOTLP()

	return []api.Topology{topology, monitoringTopology}
}
