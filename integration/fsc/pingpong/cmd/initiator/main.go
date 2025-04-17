/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong/mock"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
)

func main() {
	node := fscnode.New()
	node.Execute(func() error {
		if err := node.RegisterFactory("init", &pingpong.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := node.RegisterFactory("mockInit", &mock.InitiatorViewFactory{}); err != nil {
			return err
		}
		if err := node.RegisterFactory("stream", &pingpong.StreamerViewFactory{}); err != nil {
			return err
		}
		return nil
	})
}
