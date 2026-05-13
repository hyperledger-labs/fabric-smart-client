/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli/validate"
	"github.com/hyperledger-labs/fabric-smart-client/cmd/fsccli/version"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/artifactgen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/cryptogen"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/cmd/hsm"
	view "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
)

const CmdRoot = "fsccli"

// The main command describes the service and
// defaults to printing the help message.
var mainCmd = &cobra.Command{Use: "fsccli"}

func init() {
	mainCmd.AddCommand(artifactgen.NewCmd())
	mainCmd.AddCommand(cryptogen.NewCmd())
	mainCmd.AddCommand(view.NewCmd())
	mainCmd.AddCommand(hsm.NewCmd())
	mainCmd.AddCommand(validate.NewCmd())
	mainCmd.AddCommand(version.Cmd())

	// For environment variables with FSCCLI_ prefix.
	// This ensures that FSCCLI_TOPOLOGY correctly maps to the --topology flag, etc.
	cobra.OnInitialize(func() {
		k := koanf.New(".")
		if err := k.Load(env.Provider("", env.Opt{
			Prefix: "FSCCLI_",
			TransformFunc: func(k, v string) (string, any) {
				return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(k, "FSCCLI_")), "_", "-"), v
			},
		}), nil); err == nil {
			bindFlags(mainCmd, k)
		} else {
			panic(err)
		}
	})
}

func main() {
	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

func bindFlags(cmd *cobra.Command, k *koanf.Koanf) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Changed && k.Exists(f.Name) {
			if err := f.Value.Set(k.String(f.Name)); err != nil {
				panic(err)
			}
		}
	})
	for _, child := range cmd.Commands() {
		bindFlags(child, k)
	}
}
