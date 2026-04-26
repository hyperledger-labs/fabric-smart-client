/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package validate

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// Cmd returns the Cobra command used to validate an FSC node configuration.
func Cmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate-config",
		Short: "Validate the fabric smart client node configuration.",
		Long:  "Validate the fabric smart client node configuration and report the file that was loaded.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.Errorf("trailing args detected")
			}
			cmd.SilenceUsage = true

			usedConfig, err := Validate(configPath)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "configuration is valid: %s\n", usedConfig)
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config-path", "", "Path to a directory containing core.yaml")

	return cmd
}

// Validate loads the FSC node configuration and returns the concrete file used.
func Validate(configPath string) (string, error) {
	provider, err := config.NewProvider(configPath)
	if err != nil {
		return "", err
	}
	if len(provider.ID()) == 0 {
		return "", errors.New("missing fsc.id in configuration")
	}

	return provider.ConfigFileUsed(), nil
}
