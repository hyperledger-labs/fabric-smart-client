/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	_ "net/http/pprof"
	"os"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli/validate"
	"github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli/version"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/artifactgen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/hsm"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
)

func main() {
	mainCmd := &cobra.Command{Use: "fsccli"}

	mainCmd.AddCommand(artifactgen.NewCmd())
	mainCmd.AddCommand(cryptogen.NewCmd())
	mainCmd.AddCommand(view.NewCmd())
	mainCmd.AddCommand(hsm.NewCmd())
	mainCmd.AddCommand(validate.NewCmd())
	mainCmd.AddCommand(version.Cmd())

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}
