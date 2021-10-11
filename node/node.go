/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	node2 "github.com/hyperledger-labs/fabric-smart-client/node/node"
	"github.com/hyperledger-labs/fabric-smart-client/node/version"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	node3 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fsc")

type ExecuteCallbackFunc = func() error

type FabricSmartClient interface {
	Start() error
	Stop()
	InstallSDK(p api.SDK) error
	GetService(v interface{}) (interface{}, error)
	RegisterService(service interface{}) error
	RegisterFactory(id string, factory api.Factory) error
	RegisterResponder(responder view.View, initiatedBy view.View)
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View)
	ResolveIdentities(endpoints ...string) ([]view.Identity, error)
}

type node struct {
	FabricSmartClient

	mainCmd             *cobra.Command
	callbackChannel     chan error
	executeCallbackFunc ExecuteCallbackFunc
}

func New() *node {
	return NewFromConfPath("")
}

func NewFromConfPath(confPath string) *node {
	mainCmd := &cobra.Command{Use: "peer"}
	node := &node{
		FabricSmartClient: node3.NewFromConfPath(confPath),
		mainCmd:           mainCmd,
		callbackChannel:   make(chan error, 1),
	}

	// Define command-line flags that are valid for all peer commands and
	// subcommands.
	mainFlags := mainCmd.PersistentFlags()

	mainFlags.String("logging-level", "", "Legacy logging level flag")
	viper.BindPFlag("logging_level", mainFlags.Lookup("logging-level"))
	mainFlags.MarkHidden("logging-level")

	mainCmd.AddCommand(version.Cmd())
	mainCmd.AddCommand(node2.Cmd(node))

	return node
}

func (n *node) Callback() chan<- error {
	return n.callbackChannel
}

func (n *node) Execute(executeCallbackFunc ExecuteCallbackFunc) {
	n.executeCallbackFunc = executeCallbackFunc
	go n.listen()
	if n.mainCmd.Execute() != nil {
		os.Exit(1)
	}
}

func (n *node) listen() {
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
