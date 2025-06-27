/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/node/start"
	"github.com/hyperledger-labs/fabric-smart-client/node/version"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/spf13/cobra"
)

var logger = logging.MustGetLogger()

type ExecuteCallbackFunc = func() error

type FSCNode interface {
	ID() string
	Start() error
	Stop()
	InstallSDK(p node.SDK) error
	GetService(v interface{}) (interface{}, error)
	RegisterService(service interface{}) error
}

// Node is a cobra based application that offers the following commands:
// - `peer start` to instantiate and start the Fabric Smart Client stack.
// - `version` to get the version of the executed code.
type Node struct {
	FSCNode

	mainCmd             *cobra.Command
	callbackChannel     chan error
	executeCallbackFunc ExecuteCallbackFunc
}

// New returns a new instance of Node from the default configuration path.
func New() *Node {
	return NewWithConfPath("")
}

// NewWithConfPath returns a new instance of Node whose configuration is loaded from the passed path.
func NewWithConfPath(confPath string) *Node {
	return newWithFSCNode(node.NewFromConfPath(confPath))
}

func newWithFSCNode(fscNode FSCNode) *Node {
	mainCmd := &cobra.Command{Use: "peer"}
	node := &Node{
		FSCNode:         fscNode,
		mainCmd:         mainCmd,
		callbackChannel: make(chan error, 1),
	}

	mainCmd.AddCommand(version.Cmd())
	mainCmd.AddCommand(start.Cmd(node))

	return node
}

func (n *Node) Callback() chan<- error {
	return n.callbackChannel
}

func (n *Node) Execute(executeCallbackFunc ExecuteCallbackFunc) {
	n.executeCallbackFunc = executeCallbackFunc
	go n.listen()
	if n.mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

func (n *Node) listen() {
	logger.Debugf("Wait for callback signal")
	err := <-n.callbackChannel
	logger.Debugf("Callback signal came with err [%s]", err)
	if err != nil {
		panic(err)
	}
	if n.executeCallbackFunc != nil {
		logger.Debugf("Calling callback...")
		err = n.executeCallbackFunc()
		if err != nil {
			panic(err)
		}
	}
}
