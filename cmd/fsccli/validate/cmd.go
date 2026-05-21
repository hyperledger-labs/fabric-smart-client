/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// NewCmd returns the Cobra Command for validation related utilities.
func NewCmd() *cobra.Command {
	rootCommand := &cobra.Command{
		Use:   "validate",
		Short: "Validate FSC inputs.",
		Long:  `Validate FSC configuration and related inputs before starting a node.`,
	}

	rootCommand.AddCommand(newConfigCmd())

	return rootCommand
}

func newConfigCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Validate an FSC node configuration.",
		Long:  `Validate a core.yaml configuration directory before starting a node.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.Errorf("trailing args detected")
			}
			cmd.SilenceUsage = true

			report, err := ValidateConfig(configPath)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), report.String())
			return err
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&configPath, "path", "p", "./", "path to the directory containing core.yaml")

	return cmd
}
