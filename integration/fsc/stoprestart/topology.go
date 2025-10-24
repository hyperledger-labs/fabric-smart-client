/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package stoprestart

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

func Topology(commType fsc.P2PCommunicationType, replicationOpts *integration.ReplicationOptions) []api.Topology {
	// Create an empty FSC topology
	topology := fsc.NewTopology()
	topology.P2PCommunicationType = commType
	topology.EnableTracing(tracing.Otlp)
	topology.EnablePrometheusMetrics()

	topology.AddNodeByName("alice").
		RegisterViewFactory("init", &InitiatorViewFactory{}).
		AddOptions(replicationOpts.For("alice")...)

	topology.AddNodeByName("bob").
		RegisterResponder(&Responder{}, &Initiator{}).
		AddOptions(fsc.WithAlias("bob_alias")).
		AddOptions(replicationOpts.For("bob")...)

	topology.AddSDK(&viewsdk.SDK{})

	monitoringTopology := monitoring.NewTopology()
	// monitoringTopology.EnablePrometheusGrafana()
	monitoringTopology.EnableOTLP()

	return []api.Topology{topology, monitoringTopology}
}
