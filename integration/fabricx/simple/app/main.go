/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/network"
	nwofabricx "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabricx"
	nwofsc "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
	"github.com/onsi/gomega"
)

func main() {
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		panic(message)
	})

	m := cmd.NewMain("Simple network", "0.1")
	mainCmd := m.Cmd()
	network.StartCMDPostNew = func(infrastructure *integration.Infrastructure) error {
		infrastructure.RegisterPlatformFactory(nwofabricx.NewPlatformFactory())
		return nil
	}

	mainCmd.AddCommand(network.NewCmd(simple.Topology(&simple.SDK{}, nwofsc.WebSocket)...))

	mainCmd.AddCommand(view.NewCmd())
	m.Execute()
}
