/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"

func main() {
	path := "set the path"
	weaver.RunRelayFabricDriver(
		"alpha",
		"localhost", "20040",
		"localhost", "20041",
		"",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/cp.json",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/config.json",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/wallet-alpha",
	)
	weaver.RunRelayServer(
		"alpha",
		path+"/weaver/relay/server/Fabric_alpha/server.toml",
		"20040",
	)
}
