/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"os"

	node2 "github.com/hyperledger-labs/fabric-smart-client/node/node"
	"github.com/hyperledger-labs/fabric-smart-client/node/version"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	node3 "github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/spf13/cobra"
)

var logger = logging.MustGetLogger()

type ExecuteCallbackFunc = func() error

type FabricSmartClient interface {
	Start() error
	Stop()
	InstallSDK(p api.SDK) error
	ConfigService() node3.ConfigService
	Registry() node3.Registry
	GetService(v interface{}) (interface{}, error)
	RegisterService(service interface{}) error
	RegisterFactory(id string, factory api.Factory) error
	RegisterResponder(responder view.View, initiatedBy interface{}) error
	RegisterResponderWithIdentity(responder view.View, id view.Identity, initiatedBy view.View) error
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
	n := node3.NewEmpty(confPath)
	n.AddSDK(sdk.NewSDK(n.Registry()))
	return newFromFsc(n)
}

func newFromFsc(fscNode FabricSmartClient) *node {
	mainCmd := &cobra.Command{Use: "peer"}
	node := &node{
		FabricSmartClient: fscNode,
		mainCmd:           mainCmd,
		callbackChannel:   make(chan error, 1),
	}

	mainCmd.AddCommand(version.Cmd())
	mainCmd.AddCommand(node2.Cmd(node))

	return node
}

func NewEmpty(confPath string) *node {
	return newFromFsc(node3.NewEmpty(confPath))
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
