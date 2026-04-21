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
	nodeCmdDes   = "Operate a fabric smart client node: start."
)

type Node interface {
	ID() string
	Start() error
	Stop()
	Callback() chan<- error
}

var (
	logger = logging.MustGetLogger()
	node   Node
)

// Cmd returns the cobra command for Node
func Cmd(n Node) *cobra.Command {
	node = n

	nodeCmd.AddCommand(startCmd())
	return nodeCmd
}

var nodeCmd = &cobra.Command{
	Use:   nodeFuncName,
	Short: fmt.Sprint(nodeCmdDes),
	Long:  fmt.Sprint(nodeCmdDes),
}

func startCmd() *cobra.Command {
	nodeStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the fabric smart client node.",
		Long:  `Starts the fabric smart client node that interacts with the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.Errorf("trailing args detected")
			}
			cmd.SilenceUsage = true
			return serve()
		},
	}
	return nodeStartCmd
}

func serve() error {
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
			callback(err)
			return err
		}
		// start profiler
		if err := profiler.Start(); err != nil {
			logger.Errorf("error starting profiler: [%s]", err)
			callback(err)
			return err
		}

		defer profiler.Stop()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := node.Start(); err != nil {
		logger.Errorf("Failed starting platform [%s]", err)
		callback(err)
		return err
	}

	callback(nil)

	logger.Infof("Started peer with ID=[%s]", node.ID())

	<-ctx.Done()
	logger.Infof("Received signal, exiting...")
	node.Stop()

	return nil
}

func callback(err error) {
	node.Callback() <- err
}
