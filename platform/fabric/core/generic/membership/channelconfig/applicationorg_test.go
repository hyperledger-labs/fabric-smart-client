/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestApplicationOrgInterface(t *testing.T) {
	t.Parallel()
	_ = ApplicationOrg(&ApplicationOrgConfig{})
}

func TestNewApplicationOrgConfig(t *testing.T) {
	t.Parallel()

	t.Run("SubGroupError", func(t *testing.T) {
		t.Parallel()
		_, err := NewApplicationOrgConfig("org1", &cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"sub": {}}}, nil)
		require.ErrorContains(t, err, "does not allow sub-groups")
	})

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		_, err := NewApplicationOrgConfig("org1", &cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}, nil)
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		conf, err := msp.GetLocalMspConfig(getDevMspDir(), nil, "SampleOrg")
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())

		orgGroup := &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				MSPKey:         {Value: protoutil.MarshalOrPanic(MSPValue(conf).Value())},
				AnchorPeersKey: {Value: protoutil.MarshalOrPanic(AnchorPeersValue([]*pb.AnchorPeer{{Host: "p", Port: 7051}}).Value())},
			},
		}
		aoc, err := NewApplicationOrgConfig("org1", orgGroup, mspConfigHandler)
		require.NoError(t, err)
		require.Len(t, aoc.AnchorPeers(), 1)
		require.Equal(t, "p", aoc.AnchorPeers()[0].Host)
		require.Equal(t, "SampleOrg", aoc.MSPID())
	})
}
