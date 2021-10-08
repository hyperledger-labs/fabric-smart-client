/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"os"

	"github.com/hyperledger/fabric/cmd/common"

	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
)

func main() {
	cli := common.NewCLI("sc", "Command line client for Fabric Smart Client")
	view.RegisterViewCommand(cli)
	cli.Run(os.Args[1:])
}
