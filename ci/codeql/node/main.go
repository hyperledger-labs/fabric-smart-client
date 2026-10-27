/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	_ "net/http/pprof"

	"github.com/hyperledger-labs/fabric-smart-client/node"
)

func main() {
	// Instantiate node and execute
	node := node.New()
	node.Execute(nil)
}
