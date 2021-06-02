/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hyperledger/fabric/common/flogging"
	"github.com/spf13/cobra"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a fabfsc node: start."
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
		Long:  `Starts a fabric smart client node that interacts with the network.`,
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
	if err := node.Start(); err != nil {
		logger.Errorf("Failed starting platform [%s]", err)
		callback(err)
		return err
	}
	callback(nil)

	logger.Debugf("Block until signal")
	serve := make(chan error, 10)
	go handleSignals(addPlatformSignals(map[os.Signal]func(){
		syscall.SIGINT:  func() { node.Stop(); serve <- nil },
		syscall.SIGTERM: func() { node.Stop(); serve <- nil },
		syscall.SIGSTOP: func() { node.Stop(); serve <- nil },
	}))
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
