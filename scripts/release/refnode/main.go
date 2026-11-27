/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Command refnode is the reference FSC binary of the release tooling. It installs the Chaguar
// SDK, which builds the Fabric and view SDKs, so `refnode version --json` reports every FSC
// store. It is never started.
package main

import (
	"log"

	"github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/node/cli"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

func main() {
	n := node.NewFromConfPath("")
	if err := n.InstallSDK(fabricx.NewSDK(n)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	cli.Execute(n, nil)
}
