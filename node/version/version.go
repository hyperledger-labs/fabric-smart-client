/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"cmp"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/buildinfo"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// ProgramName is the program name
const ProgramName = "node"

// Cmd returns the Cobra Command for Version
func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the fabric smart client version.",
		Long:  `Print current version of the fabric smart client.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			fmt.Print(GetInfo())
			return nil
		},
	}
}

// GetInfo returns the program name, the Go toolchain, the platform, and the build info Go
// embeds in the binary: the FSC version, the application module and the VCS revision.
func GetInfo() string {
	return format(buildinfo.Get())
}

func format(info buildinfo.Info) string {
	app := cmp.Or(strings.TrimSpace(info.AppModule+" "+info.AppVersion), "unknown")
	revision := cmp.Or(info.Revision, "unknown")
	if info.Modified {
		revision += " (modified)"
	}
	return fmt.Sprintf("%s:\n Go version: %s\n OS/Arch: %s/%s\n"+
		" FSC version: %s\n Application: %s\n Revision: %s\n",
		ProgramName, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		info.FSCVersion, app, revision)
}
