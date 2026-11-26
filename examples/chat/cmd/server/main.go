/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"log"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chat"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func main() {
	node := fscnode.New()
	if err := node.InstallSDK(viewsdk.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(func() error {
		return chat.RegisterServer(node)
	})
}
