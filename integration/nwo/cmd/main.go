/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Main struct {
	ProgramName string
	Version     string
	mainCmd     *cobra.Command
}

func NewMain(pName, version string) *Main {
	mainCmd := &cobra.Command{Use: strings.ToLower(pName)}
	mainCmd.AddCommand(VersionCmd(pName))
	return &Main{
		ProgramName: pName,
		Version:     version,
		mainCmd:     mainCmd,
	}
}

func (m *Main) Cmd() *cobra.Command {
	return m.mainCmd
}

func (m *Main) Execute() {
	// For environment variables.
	viper.SetEnvPrefix(m.ProgramName)
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)

	// Define command-line flags that are valid for all peer commands and
	// subcommands.
	mainFlags := m.mainCmd.PersistentFlags()

	mainFlags.String("logging-level", "", "Legacy logging level flag")
	viper.BindPFlag("logging_level", mainFlags.Lookup("logging-level"))
	mainFlags.MarkHidden("logging-level")

	// On failure Cobra prints the usage message and error string, so we only
	// need to exit with a non-0 status
	if m.mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

// VersionCmd returns the Cobra Command for Version
func VersionCmd(pName string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print iou version.",
		Long:  `Print current version of IOU CLI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			fmt.Printf("%s:\n Go version: %s\n  OS/Arch: %s/%s\n",
				pName, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
