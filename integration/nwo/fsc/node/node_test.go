/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/mocks/sdk1"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/mocks/sdk2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/mocks/sdk3"
	"github.com/stretchr/testify/assert"
)

func TestNode_AddSDKWithBase(t *testing.T) {
	node := NewNode("pineapple")
	n := node.AddSDKWithBase(&sdk1.DummySDK{}, &sdk2.DummySDK{}, &sdk3.DummySDK{})
	assert.NotNil(t, n)
	assert.Equal(t, "sdk2.NewFrom(sdk3.NewFrom(sdk1.NewDummySDK(n)))", n.SDKs[0].Type)

	node = NewNode("pineapple")
	n = node.AddSDKWithBase(&sdk1.DummySDK{}, &sdk2.DummySDK{})
	assert.NotNil(t, n)
	assert.Equal(t, "sdk2.NewFrom(sdk1.NewDummySDK(n))", n.SDKs[0].Type)

	node = NewNode("pineapple")
	n = node.AddSDKWithBase(&sdk3.DummySDK{})
	assert.NotNil(t, n)
	assert.Equal(t, "sdk3.NewDummySDK(n)", n.SDKs[0].Type)
}
