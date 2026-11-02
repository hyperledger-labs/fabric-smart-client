/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package start

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/node/start/profile"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a fabric smart client node: start, migrate."
)

type Node interface {
	ID() string
	Start() error
	Stop()
	Callback() chan<- error
}

var logger = logging.MustGetLogger()

// Cmd returns the cobra command for Node
func Cmd(n Node) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   nodeFuncName,
		Short: fmt.Sprint(nodeCmdDes),
		Long:  fmt.Sprint(nodeCmdDes),
	}
	nodeCmd.AddCommand(startCmd(n))
	if m, ok := n.(Migrator); ok {
		nodeCmd.AddCommand(migrateCmd(m))
	}
	return nodeCmd
}

// Migrator prepares the schema of every store a node opens, without starting
// the node.
type Migrator interface {
	Migrate(ctx context.Context, checkOnly bool) error
}

func migrateCmd(m Migrator) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Runs the pending schema steps of every store and exits.",
		Long: `Opens every store the node uses, without starting servers, P2P or delivery, and runs
their pending schema steps. With --check it runs no step and exits non-zero if a store has
steps pending. The command reads the node's full configuration, including its identities.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			cmd.SilenceUsage = true
			// No signal handler: a migrator waiting for the lock must die on SIGTERM.
			if err := m.Migrate(cmd.Context(), check); err != nil {
				return err
			}
			if check {
				logger.Infof("no schema steps pending")
			} else {
				logger.Infof("schema migration done")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "run no step; exit non-zero if a store has steps pending")
	return cmd
}

func startCmd(n Node) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Starts the fabric smart client node.",
		Long:  `Starts the fabric smart client node that interacts with the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			cmd.SilenceUsage = true
			return serve(n)
		},
	}
}

func serve(n Node) error {
	// config path
	configPath := os.Getenv("FSCNODE_CFG_PATH")
	if configPath == "" {
		configPath = "./"
	}

	// profile
	enableProfile := false
	enableProfileStr := os.Getenv("FSCNODE_PROFILER")
	if len(enableProfileStr) != 0 {
		var err error
		enableProfile, err = strconv.ParseBool(enableProfileStr)
		if err != nil {
			logger.Infof("Error parsing boolean environment variable FSCNODE_PROFILER: %s\n", err.Error())
		}
	}

	if enableProfile {
		logger.Infof("Profiling enabled")
		profiler, err := profile.New(profile.WithPath(configPath), profile.WithAll())
		if err != nil {
			logger.Errorf("error creating profiler: [%s]", err)
			n.Callback() <- err
			return err
		}
		// start profiler
		if err := profiler.Start(); err != nil {
			logger.Errorf("error starting profiler: [%s]", err)
			n.Callback() <- err
			return err
		}

		defer profiler.Stop()
	}

	// Not signal.NotifyContext: it reports only the first signal, and a second one must reach
	// n.Stop to skip the rest of the drain.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	if err := n.Start(); err != nil {
		logger.Errorf("Failed starting platform [%s]", err)
		n.Callback() <- err
		return err
	}

	n.Callback() <- nil

	logger.Infof("Started peer with ID=[%s]", n.ID())

	<-signals
	logger.Infof("Received signal, exiting...")
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-signals:
			logger.Infof("Received second signal, skipping the rest of the drain...")
			n.Stop()
		case <-stopped:
		}
	}()
	n.Stop()

	return nil
}
