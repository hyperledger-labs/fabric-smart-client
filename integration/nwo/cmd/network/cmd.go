/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"github.com/pkg/errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
)

var (
	path string
)

// NewCmd returns the Cobra Command for the network subcommands
func NewCmd(postNew, postStart CallbackFunc, topologies ...api.Topology) *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "network",
		Short: "Gen crypto artifacts.",
		Long:  `Generate crypto material.`,
	}

	rootCommand.AddCommand(
		GenerateCmd(topologies...),
		CleanCmd(),
		StartCmd(postNew, postStart, topologies...),
	)

	return rootCommand
}

// GenerateCmd returns the Cobra Command for Generate
func GenerateCmd(topologies ...api.Topology) *cobra.Command {
	cmd := &cobra.Command{
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
	flags := cmd.Flags()
	flags.StringVarP(&path, "path", "p", "", "where to store the generated network artifacts")

	return cmd
}

// Generate returns version information for the peer
func Generate(topologies ...api.Topology) error {
	_, err := integration.GenerateAt(20000, path, true, topologies...)
	return err
}

// CleanCmd returns the Cobra Command for Clean
func CleanCmd() *cobra.Command {
	cmd := &cobra.Command{
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
	flags := cmd.Flags()
	flags.StringVarP(&path, "path", "p", "", "where to store the generated network artifacts")

	return cmd
}

// Clean returns version information for the peer
func Clean() error {
	// delete artifacts folder
	err := os.RemoveAll(path)
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
func StartCmd(postNew, postStart CallbackFunc, topologies ...api.Topology) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start Artifacts.",
		Long:  `Start Artifacts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected")
			}
			// Parsing of the command line is done so silence cmd usage
			cmd.SilenceUsage = true
			return Start(postNew, postStart, topologies...)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&path, "path", "p", "", "where to store the generated network artifacts")

	return cmd
}

type CallbackFunc func(*integration.Infrastructure) error

// Start returns version information for the peer
func Start(postNew, postStart CallbackFunc, topologies ...api.Topology) error {
	// if ./artifacts exists, then load. Otherwise, create new artifacts
	var ii *integration.Infrastructure
	init := true
	ii, err := integration.New(20000, path, topologies...)
	if err != nil {
		return errors.WithMessage(err, "failed to create new infrastructure")
	}
	ii.EnableRaceDetector()
	if postNew != nil {
		err = postNew(ii)
		if err != nil {
			return errors.WithMessage(err, "failed to post new")
		}
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		ii.Generate()
	} else {
		init = false
		ii.Load()
	}

	ii.DeleteOnStop = false
	ii.Start()
	if init && postStart != nil {
		err = postStart(ii)
		if err != nil {
			return errors.WithMessage(err, "failed to post start")
		}
	}

	return ii.Serve()
}
