/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package version

import (
	"cmp"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// ProgramName is the program name
const ProgramName = "node"

// fscModule is the module path of the Fabric Smart Client.
const fscModule = "github.com/hyperledger-labs/fabric-smart-client"

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
	return format(fromBuildInfo(debug.ReadBuildInfo()))
}

// buildInfo is what GetInfo reports about the binary's modules.
type buildInfo struct {
	// fscVersion is the FSC module's version: the dependency's version, or the main
	// module's when FSC is the main module. A replaced dependency reports the replacement's
	// version. It is "(devel)" when FSC is replaced by a local directory or the binary
	// carries no build info.
	fscVersion string
	// appModule and appVersion are the main module's path and version. A main module
	// built from a checkout without a VCS tag reports a pseudo-version or "(devel)".
	appModule, appVersion string
	// revision and modified are the main module's vcs.revision and vcs.modified build
	// settings; revision is empty when the build recorded no VCS information.
	revision string
	modified bool
}

func fromBuildInfo(bi *debug.BuildInfo, ok bool) buildInfo {
	// "(devel)" is what Go reports for a module built from a local directory.
	info := buildInfo{fscVersion: "(devel)"}
	if !ok {
		return info
	}
	info.appModule, info.appVersion = bi.Main.Path, bi.Main.Version
	if bi.Main.Path == fscModule {
		info.fscVersion = bi.Main.Version
	}
	for _, d := range bi.Deps {
		if d.Path == fscModule {
			if d.Replace != nil {
				d = d.Replace
			}
			info.fscVersion = cmp.Or(d.Version, info.fscVersion)
		}
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.revision = s.Value
		case "vcs.modified":
			info.modified = s.Value == "true"
		}
	}
	return info
}

func format(info buildInfo) string {
	app := cmp.Or(strings.TrimSpace(info.appModule+" "+info.appVersion), "unknown")
	revision := cmp.Or(info.revision, "unknown")
	if info.modified {
		revision += " (modified)"
	}
	return fmt.Sprintf("%s:\n Go version: %s\n OS/Arch: %s/%s\n"+
		" FSC version: %s\n Application: %s\n Revision: %s\n",
		ProgramName, runtime.Version(), runtime.GOOS, runtime.GOARCH,
		info.fscVersion, app, revision)
}
