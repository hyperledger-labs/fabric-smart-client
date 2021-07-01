/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func main() {
	configs := make(view.Configs, 2)
	configs[0] = view.Config{
		ID: "Alice",
		FSCNode: &grpc.ConnectionConfig{
			Address: "127.0.0.1:7051",
		},
	}
	configs[1] = view.Config{
		ID: "Bob",
		FSCNode: &grpc.ConnectionConfig{
			Address: "127.0.0.1:8051",
		},
	}
	raw, _ := configs.ToJSon()

	configs2 := &view.Configs{}
	err := json.Unmarshal(raw, configs2)
	if err != nil {
		panic(err)
	}
	raw2, _ := configs2.ToJSon()
	if !bytes.Equal(raw, raw2) {
		panic("arrays are different")
	}
	fmt.Println(string(raw))
}
