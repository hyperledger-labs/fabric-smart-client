/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"fmt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
)

type (
	// CallbackFunc is the type of the callback function
	CallbackFunc func(*integration.Infrastructure) error

	Topologies map[string][]api.Topology
)

var (
	logger = flogging.MustGetLogger("nwo.network")

	path     string
	topology string
	// StartCMDPostNew is executed after the testing infrastructure is created
	StartCMDPostNew CallbackFunc
	// StartCMDPostStart is executed after the testing infrastructure is started
	StartCMDPostStart CallbackFunc
)

// NewCmd returns the Cobra Command for the network subcommands
func NewCmd(topologies ...api.Topology) *cobra.Command {
	return NewCmdWithMultipleTopologies(map[string][]api.Topology{
		"default": topologies,
	})
}

// NewCmdWithMultipleTopologies returns the Cobra Command for the network subcommands
func NewCmdWithMultipleTopologies(topologies Topologies) *cobra.Command {
	// Set the flags on the node start command.
	rootCommand := &cobra.Command{
		Use:   "network",
		Short: "Gen crypto artifacts.",
		Long:  `Generate crypto material.`,
	}

	rootCommand.AddCommand(
		GenerateCmd(topologies),
		CleanCmd(),
		StartCmd(topologies),
	)

	return rootCommand
}

// GenerateCmd returns the Cobra Command for Generate
func GenerateCmd(topologies Topologies) *cobra.Command {
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
			return Generate(topologies)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&path, "path", "p", "", "where to store the generated network artifacts")
	flags.StringVarP(&topology, "topology", "t", "default", "topology to use (in case multiple topologies are provided)")

	return cmd
}

// Generate returns version information for the peer
func Generate(topologies Topologies) error {
	ii, err := integration.New(20000, path, topologies[topology]...)
	if err != nil {
		return errors.WithMessage(err, "failed to create new infrastructure")
	}
	ii.EnableRaceDetector()
	if StartCMDPostNew != nil {
		err = StartCMDPostNew(ii)
		if err != nil {
			return errors.WithMessage(err, "failed to post new")
		}
	}
	ii.Generate()
	return nil
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
func StartCmd(topologies Topologies) *cobra.Command {
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
			return Start(topologies)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&path, "path", "p", "", "where to store the generated network artifacts")
	flags.StringVarP(&topology, "topology", "t", "default", "topology to use (in case multiple topologies are provided)")

	return cmd
}

// Start returns version information for the peer
func Start(topologies Topologies) error {
	logger.Infof(" ____    _____      _      ____    _____")
	logger.Infof("/ ___|  |_   _|    / \\    |  _ \\  |_   _|")
	logger.Infof("\\___ \\    | |     / _ \\   | |_) |   | |")
	logger.Infof("___) |    | |    / ___ \\  |  _ <    | |")
	logger.Infof("|____/    |_|   /_/   \\_\\ |_| \\_\\   |_|")

	// if ./artifacts exists, then load. Otherwise, create new artifacts
	var ii *integration.Infrastructure
	_, err := os.Stat(path)
	init := os.IsNotExist(err)

	ii, err = integration.New(20000, path, topologies[topology]...)
	if err != nil {
		return errors.WithMessage(err, "failed to create new infrastructure")
	}
	ii.EnableRaceDetector()
	if StartCMDPostNew != nil {
		err = StartCMDPostNew(ii)
		if err != nil {
			return errors.WithMessage(err, "failed to post new")
		}
	}

	if init {
		ii.Generate()
	} else {
		ii.Load()
	}

	ii.DeleteOnStop = false
	ii.Start()
	if StartCMDPostStart != nil {
		err = StartCMDPostStart(ii)
		if err != nil {
			return errors.WithMessage(err, "failed to post start")
		}
	}
	defer ii.Stop()

	logger.Infof(" _____   _   _   ____")
	logger.Infof("| ____| | \\ | | |  _ \\")
	logger.Infof("|  _|   |  \\| | | | | |")
	logger.Infof("| |___  | |\\  | | |_| |")
	logger.Infof("|_____| |_| \\_| |____/")

	return ii.Serve()
}
