/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/fake/sdk1"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/fake/sdk2"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc/node/fake/sdk3"
)

func TestNode_AddSDKWithBase(t *testing.T) {
	t.Parallel()
	node := NewNode("pineapple")
	n := node.AddSDKWithBase(&sdk1.DummySDK{}, &sdk2.DummySDK{}, &sdk3.DummySDK{})
	require.NotNil(t, n)
	require.Equal(t, "sdk2.NewFrom(sdk3.NewFrom(sdk1.NewDummySDK(n)))", n.SDKs[0].Type)

	node = NewNode("pineapple")
	n = node.AddSDKWithBase(&sdk1.DummySDK{}, &sdk2.DummySDK{})
	require.NotNil(t, n)
	require.Equal(t, "sdk2.NewFrom(sdk1.NewDummySDK(n))", n.SDKs[0].Type)

	node = NewNode("pineapple")
	n = node.AddSDKWithBase(&sdk3.DummySDK{})
	require.NotNil(t, n)
	require.Equal(t, "sdk3.NewDummySDK(n)", n.SDKs[0].Type)
}
