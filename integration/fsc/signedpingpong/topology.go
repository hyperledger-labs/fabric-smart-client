/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signedpingpong

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func Topology(replicationOpts *integration.ReplicationOptions) []api.Topology {
	topology := fsc.NewTopology()

	topology.AddNodeByName("alice").
		RegisterViewFactory("signedInit", &AliceInitiatorViewFactory{}).
		AddOptions(fsc.WithAlias("alice")).
		AddOptions(replicationOpts.For("alice")...)

	topology.AddNodeByName("bob").
		RegisterResponder(&BobResponder{}, &AliceInitiator{}).
		AddOptions(fsc.WithAlias("bob")).
		AddOptions(replicationOpts.For("bob")...)

	topology.AddSDK(&viewsdk.SDK{})

	return []api.Topology{topology}
}
