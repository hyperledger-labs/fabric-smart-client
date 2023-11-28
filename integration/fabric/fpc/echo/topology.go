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
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk"
)

func Topology() []api.Topology {
	// Create an empty fabric topology
	fabricTopology := fabric.NewDefaultTopology()
	// Note that Idemix is currently not supported by FPC
	// fabricTopology.EnableIdemix()
	// Add two organizations
	fabricTopology.AddOrganizationsByName("Org1", "Org2")
	// Add a standard chaincode
	// fabricTopology.AddNamespaceWithUnanimity("mycc", "Org1", "Org2")
	// Add an FPC by passing chaincode's id and docker image
	fabricTopology.AddFPC("echo", "fpc/fpc-echo").AddPostRunInvocation(
		"init", "init", []byte("init"),
	)

	// Create an empty FSC topology
	fscTopology := fsc.NewTopology()
	fscTopology.SetLogging("debug", "")

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

	// Add Fabric SDK to FSC Nodes
	fscTopology.AddSDK(&fabric2.SDK{})

	return []api.Topology{fabricTopology, fscTopology}
}
