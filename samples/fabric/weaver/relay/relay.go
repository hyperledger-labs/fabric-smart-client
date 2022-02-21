/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/network"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/weaver/relay/topology"
)

func main() {
	m := cmd.NewMain("Fabric2Fabric Interoperability", "0.1")
	mainCmd := m.Cmd()
	mainCmd.AddCommand(network.NewCmd(topology.Topology()...))
	mainCmd.AddCommand(view.NewCmd())
	m.Execute()
}
