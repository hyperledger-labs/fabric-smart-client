/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cryptogen

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCmd returns the Cobra Command for the artifactsgen
func NewCmd() *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "cryptogen",
		Short: "Gen crypto artifacts.",
		Long:  `Generate crypto material.`,
	}

	rootCommand.AddCommand(
		newGenCmd(),
		newShowTemplateCmd(),
	)

	return rootCommand
}

var (
	outputDir     string
	genConfigFile string

	// ext           = app.Command("extend", "Extend existing network")
	// inputDir      = ext.Flag("input", "The input directory in which existing network place").Default("crypto-config").String()
	// extConfigFile = ext.Flag("config", "The configuration template to use").File()

	inputDir      string
	extConfigFile string
)

func newGenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Gen crypto artifacts.",
		Long:  `Generate crypto material.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			generate()
			return nil
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&outputDir, "output", "o", "crypto-config", "The output directory in which to place artifacts")
	flags.StringVarP(&genConfigFile, "config", "c", "", "The configuration template to use")

	return cmd
}

func newShowTemplateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "showtemplate",
		Short: "Show the default configuration template",
		Long:  `Show the default configuration template`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(defaultConfig)
			return nil
		},
	}
	return cmd
}
