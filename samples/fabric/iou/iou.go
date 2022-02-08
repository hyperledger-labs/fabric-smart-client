/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/network"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/samples/fabric/iou/topology"
)

func main() {
	m := cmd.NewMain("IOU", "0.1")
	mainCmd := m.Cmd()
	mainCmd.AddCommand(network.NewCmd(nil, nil, topology.Topology()...))
	mainCmd.AddCommand(view.NewCmd())
	m.Execute()
}
