/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package echo

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/fpc/echo/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func Topology() []api.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	fabricTopology.EnableFPC()
	fabricTopology.AddFPC("echo", "fpc/fpc-echo")

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()

	// Alice
	alice := fscTopology.AddNodeByName("alice")
	alice.AddOptions(fabric.WithOrganization("Org2"))
	alice.RegisterViewFactory("ListProvisionedEnclaves", &views.ListProvisionedEnclavesViewFactory{})
	alice.RegisterViewFactory("Echo", &views.EchoViewFactory{})

	// Bob
	bob := fscTopology.AddNodeByName("bob")
	bob.AddOptions(fabric.WithOrganization("Org2"))
	bob.RegisterViewFactory("ListProvisionedEnclaves", &views.ListProvisionedEnclavesViewFactory{})
	bob.RegisterViewFactory("Echo", &views.EchoViewFactory{})

	return []api.Topology{fabricTopology, fscTopology}
}
