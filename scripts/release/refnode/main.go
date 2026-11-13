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

	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	fabricx "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(fabricx.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(nil)
}
