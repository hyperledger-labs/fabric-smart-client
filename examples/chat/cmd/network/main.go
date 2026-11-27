/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Command network generates the configuration and crypto material of the chat network:
//
//	go run ./fsc/chat/cmd/network generate -p <dir>
package main

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chat"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

func main() {
	if network.NewCmd(chat.Topology(fsc.WebSocket)...).Execute() != nil {
		os.Exit(1)
	}
}
