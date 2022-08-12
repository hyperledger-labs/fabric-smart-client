/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hsm

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/pkcs11"
	"github.com/spf13/cobra"
)

// NewCmd returns the Cobra Command for the HSM related utilities
func NewCmd() *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "hsm",
		Short: "HSM related utils.",
		Long:  `HSM related utilities.`,
	}

	rootCommand.AddCommand(
		newListHSMSlots(),
	)

	return rootCommand
}

func newListHSMSlots() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show-slots",
		Short: "Show HSM Slots.",
		Long:  `Show HSM Slots.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			lib, pin, label, err := pkcs11.FindPKCS11Lib()
			if err != nil {
				return err
			}
			tokens, err := pkcs11.ListTokens(&config.PKCS11{
				Security: 256,
				Hash:     "SHA2",
				Library:  lib,
				Label:    label,
				Pin:      pin,
			})
			if err != nil {
				return err
			}
			fmt.Println("Tokens found:")
			for _, token := range tokens {
				fmt.Printf("%s\n", token.Label)
			}
			return nil
		},
	}
	return cmd
}
