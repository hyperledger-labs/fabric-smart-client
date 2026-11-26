/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Command client is a chat client node. "client node start" runs it headless, as nwo does;
// "client tui" runs it with the terminal user interface. Both read the node's configuration
// from FSCNODE_CFG_PATH.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chat"
	"github.com/hyperledger-labs/fabric-smart-client/examples/chat/tui"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "tui" {
		if err := runTUI(); err != nil {
			log.Fatal(err)
		}
		return
	}
	node := fscnode.New()
	if err := node.InstallSDK(viewsdk.NewSDK(node)); err != nil {
		log.Fatalf("failed installing SDK: %v", err)
	}
	node.Execute(func() error {
		return chat.RegisterClient(node, chat.NewInbox())
	})
}

// runTUI starts the node, with its logs in client.log next to its configuration, and runs
// the terminal user interface until the user quits.
func runTUI() error {
	dir := os.Getenv("FSCNODE_CFG_PATH")
	if dir == "" {
		return errors.New("set FSCNODE_CFG_PATH to the node's configuration directory")
	}
	logFile, err := os.Create(filepath.Join(dir, "client.log"))
	if err != nil {
		return errors.Wrap(err, "failed creating log file")
	}
	defer func() { _ = logFile.Close() }()
	if err := logging.Init(logging.Config{Writer: logFile}); err != nil {
		return errors.Wrap(err, "failed initializing logging")
	}

	node := fscnode.NewWithConfPath(dir)
	if err := node.InstallSDK(viewsdk.NewSDK(node)); err != nil {
		return errors.Wrap(err, "failed installing SDK")
	}
	if err := node.Start(); err != nil {
		return errors.Wrap(err, "failed starting node")
	}
	defer node.Stop()
	inbox := chat.NewInbox()
	if err := chat.RegisterClient(node, inbox); err != nil {
		return err
	}
	mgr, err := viewservice.GetManager(node)
	if err != nil {
		return err
	}
	return tui.Run(mgr, inbox, node.ID())
}
