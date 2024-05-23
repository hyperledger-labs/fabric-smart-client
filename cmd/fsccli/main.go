/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli/version"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/artifactgen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/hsm"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view/cmd"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const CmdRoot = "fsccli"

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{Use: "fsccli"}

func main() {
	// For environment variables.
	viper.SetEnvPrefix(CmdRoot)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	mainCmd.AddCommand(artifactgen.NewCmd())
	mainCmd.AddCommand(cryptogen.NewCmd())
	mainCmd.AddCommand(view.NewCmd())
	mainCmd.AddCommand(hsm.NewCmd())
	mainCmd.AddCommand(version.Cmd())

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}
