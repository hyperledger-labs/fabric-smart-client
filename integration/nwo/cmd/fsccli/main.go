/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	_ "net/http/pprof"
	"os"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/artifactgen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/cryptogen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/hsm"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/validate"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/version"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/commands/view"
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
