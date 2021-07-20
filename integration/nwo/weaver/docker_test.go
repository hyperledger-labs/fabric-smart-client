/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver_test

import (
	"testing"
	"time"

	common2 "github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/common"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/weaver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver/relay"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func TestRelayServerConnection(t *testing.T) {
	RegisterFailHandler(failMe)

	path := "/Users/adc/golang/src/github.com/hyperledger-labs/fabric-smart-client/testdata"
	platform := weaver.NewPlatform(nil, weaver.NewTopology(), nil)
	// defer platform.Cleanup()
	platform.RunRelayFabricDriver(
		"alpha",
		"127.0.0.1", "20040",
		"127.0.0.1", "20041",
		"",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/cp.json",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/config.json",
		path+"/weaver/relay/fabric-driver/Fabric_alpha/wallet-alpha",
	)
	platform.RunRelayServer(
		"alpha",
		path+"/weaver/relay/server/Fabric_alpha/server.toml",
		"20040",
	)

	time.Sleep(10 * time.Second)

	config := &relay.ClientConfig{
		ID: "test",
		RelayServer: &grpc.ConnectionConfig{
			Address:           "127.0.0.1:20040",
			ConnectionTimeout: 10 * time.Second,
			TLSEnabled:        false,
		},
	}
	c, err := relay.NewClient(config, nil, nil)
	Expect(err).ToNot(HaveOccurred())
	defer c.Close()

	ack, err := c.RequestState()
	Expect(err).ToNot(HaveOccurred())
	Expect(ack.Status).To(BeEquivalentTo(common2.Ack_ERROR))
}

func failMe(message string, callerSkip ...int) {
	panic(message)
}
