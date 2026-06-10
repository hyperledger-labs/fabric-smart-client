//go:build !pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hsm

import (
	"github.com/spf13/cobra"
)

// NewCmd returns a placeholder Cobra command when pkcs11 support is not compiled in.
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hsm",
		Short: "HSM related utils (not available; rebuild with -tags pkcs11).",
		Long:  "HSM related utilities are not available in this build. Rebuild with: go build -tags pkcs11",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
