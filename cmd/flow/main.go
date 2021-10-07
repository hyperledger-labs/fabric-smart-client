/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"

	flow "github.com/hyperledger-labs/fabric-smart-client/platform/view/cmd"
	"github.com/hyperledger/fabric/cmd/common"
)

func main() {
	cli := common.NewCLI("sc", "Command line client for Fabric smart client")
	flow.RegisterFlowCommand(cli)
	cli.Run(os.Args[1:])
}
