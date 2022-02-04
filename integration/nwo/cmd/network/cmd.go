/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/iou"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
)

// NewCmd returns the Cobra Command for the network subcommands
func NewCmd(topologies ...api.Topology) *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "network",
		Short: "Gen crypto artifacts.",
		Long:  `Generate crypto material.`,
	}

	rootCommand.AddCommand(
		GenerateCmd(topologies...),
		CleanCmd(),
		StartCmd(topologies...),
	)

	return rootCommand
}

// GenerateCmd returns the Cobra Command for Generate
func GenerateCmd(topologies ...api.Topology) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate Artifacts.",
		Long:  `Generate Artifacts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return Generate(topologies...)
		},
	}
}

// Generate returns version information for the peer
func Generate(topologies ...api.Topology) error {
	_, err := integration.GenerateAt(20000, "./artifacts", true, topologies...)
	return err
}

// CleanCmd returns the Cobra Command for Clean
func CleanCmd() *cobra.Command {
	return CleanCommand
}

var CleanCommand = &cobra.Command{
	Use:   "clean",
	Short: "Clean Artifacts.",
	Long:  `Clean Artifacts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 0 {
			return fmt.Errorf("trailing args detected")
		}
		// Parsing of the command line is done so silence cmd usage
		cmd.SilenceUsage = true
		return Clean()
	},
}

// Clean returns version information for the peer
func Clean() error {
	// delete artifacts folder
	err := os.RemoveAll("./artifacts")
	if err != nil {
		return err
	}
	// delete cmd folder
	err = os.RemoveAll("./cmd")
	if err != nil {
		return err
	}
	return nil
}

// StartCmd returns the Cobra Command for Start
func StartCmd(topologies ...api.Topology) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start Artifacts.",
		Long:  `Start Artifacts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return Start(topologies...)
		},
	}
}

// Start returns version information for the peer
func Start(topologies ...api.Topology) error {
	// if ./artifacts exists, then load. Otherwise, create new artifacts
	var ii *integration.Infrastructure
	if _, err := os.Stat("./artifacts"); os.IsNotExist(err) {
		ii, err = integration.GenerateAt(20000, "./artifacts", true, iou.Topology()...)
		if err != nil {
			return err
		}
	} else {
		ii, err = integration.Load(20000, "./artifacts", true, iou.Topology()...)
		if err != nil {
			return err
		}
	}

	ii.NWO.TerminationSignal = nil
	ii.DeleteOnStop = false
	ii.Start()
	return nil
}
