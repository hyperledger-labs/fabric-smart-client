/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/node/node/profile"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a fabric smart client node: start."
)

type Node interface {
	Start() error
	Stop()
	Callback() chan<- error
}

var (
	logger = flogging.MustGetLogger("fsc.node.start")
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
	var nodeStartCmd = &cobra.Command{
		Use:   "start",
		Short: "Starts the fabric smart client node.",
		Long:  `Starts the fabric smart client node that interacts with the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("trailing args detected")
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
		var profiler, err = profile.New(profile.WithPath(configPath), profile.WithAll())
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

	// sigup
	sighupIgnore := false
	sighupIgnoreEnv := os.Getenv("FSCNODE_SIGHUP_IGNORE")
	if len(sighupIgnoreEnv) != 0 {
		var err error
		sighupIgnore, err = strconv.ParseBool(sighupIgnoreEnv)
		if err != nil {
			logger.Infof("Error parsing boolean environment variable FSCNODE_SIGHUP_IGNORE: %s\n", err.Error())
		}
	}

	serve := make(chan error, 10)
	go handleSignals(addPlatformSignals(map[os.Signal]func(){
		syscall.SIGINT:  func() { node.Stop(); serve <- nil },
		syscall.SIGTERM: func() { node.Stop(); serve <- nil },
		syscall.SIGSTOP: func() { node.Stop(); serve <- nil },
		syscall.SIGHUP: func() {
			if sighupIgnore {
				logger.Infof("SIGHUP received, ignoring...")
			} else {
				node.Stop()
				serve <- nil
			}
		},
	}))

	if err := node.Start(); err != nil {
		logger.Errorf("Failed starting platform [%s]", err)
		callback(err)
		return err
	}
	callback(nil)

	logger.Debugf("Block until signal")
	return <-serve
}

func callback(err error) {
	node.Callback() <- err
}

func handleSignals(handlers map[os.Signal]func()) {
	var signals []os.Signal
	for sig := range handlers {
		signals = append(signals, sig)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)

	for sig := range signalChan {
		logger.Infof("Received signal: %d (%s)", sig, sig)
		handlers[sig]()
	}
}
