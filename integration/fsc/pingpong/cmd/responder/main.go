/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	fscnode "github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

func main() {
	node := fscnode.New()
	node.Execute(func() error {
		registry := view.GetRegistry(node)
		initiatorID := registry.GetIdentifier(&pingpong.Initiator{})
		if err := registry.RegisterResponder(&pingpong.Responder{}, initiatorID); err != nil {
			return err
		}
		return nil
	})
}
