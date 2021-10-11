/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/cmd/artifactsgen"
	"github.com/hyperledger-labs/fabric-smart-client/cmd/cryptogen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [view | cryptogen | artifactsgen]\n", os.Args[0])
	os.Exit(1)
}

func reArrangeArgs() {
	var args []string
	args = append(args, os.Args[0])
	args = append(args, os.Args[2:]...)
	os.Args = args
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "cryptogen":
		reArrangeArgs()
		cryptogen.Gen()
		return
	case "artifactsgen":
		reArrangeArgs()
		artifactsgen.Gen()
		return
	case "view":
		cli := common.NewCLI("sc", "Command line client for Fabric Smart Client")
		view.RegisterViewCommand(cli)
		cli.Run(os.Args[1:])
		return
	default:
		usage()
	}

}
