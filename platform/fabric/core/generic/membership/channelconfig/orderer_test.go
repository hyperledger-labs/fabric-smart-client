/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig/capabilities"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestBatchSize(t *testing.T) {
	t.Parallel()
	validMaxMessageCount := uint32(10)
	validAbsoluteMaxBytes := uint32(1000)
	validPreferredMaxBytes := uint32(500)

	oc := &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}}}
	require.NoError(t, oc.validateBatchSize(), "BatchSize was valid")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: 0, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validPreferredMaxBytes}}}
	require.Error(t, oc.validateBatchSize(), "MaxMessageCount was zero")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: 0, PreferredMaxBytes: validPreferredMaxBytes}}}
	require.Error(t, oc.validateBatchSize(), "AbsoluteMaxBytes was zero")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchSize: &ab.BatchSize{MaxMessageCount: validMaxMessageCount, AbsoluteMaxBytes: validAbsoluteMaxBytes, PreferredMaxBytes: validAbsoluteMaxBytes + 1}}}
	require.Error(t, oc.validateBatchSize(), "PreferredMaxBytes larger to AbsoluteMaxBytes")
}

func TestBatchTimeout(t *testing.T) {
	t.Parallel()
	oc := &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "1s"}}}
	require.NoError(t, oc.validateBatchTimeout(), "Valid batch timeout")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "-1s"}}}
	require.Error(t, oc.validateBatchTimeout(), "Negative batch timeout")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "0s"}}}
	require.Error(t, oc.validateBatchTimeout(), "Zero batch timeout")

	oc = &OrdererConfig{protos: &OrdererProtos{BatchTimeout: &ab.BatchTimeout{Timeout: "garbage"}}}
	require.ErrorContains(t, oc.validateBatchTimeout(), "invalid value", "Unparsable batch timeout")
}

func TestOrdererConfigAccessors(t *testing.T) {
	t.Parallel()
	consenters := []*cb.Consenter{{Id: 1, Host: "node1", Port: 7050, MspId: "org1"}}
	oc := &OrdererConfig{
		protos: &OrdererProtos{
			ConsensusType:       &ab.ConsensusType{Type: "etcdraft", Metadata: []byte("md"), State: ab.ConsensusType_STATE_MAINTENANCE},
			BatchSize:           &ab.BatchSize{MaxMessageCount: 10, AbsoluteMaxBytes: 1000, PreferredMaxBytes: 500},
			BatchTimeout:        &ab.BatchTimeout{Timeout: "2s"},
			ChannelRestrictions: &ab.ChannelRestrictions{MaxCount: 7},
			Orderers:            &cb.Orderers{ConsenterMapping: consenters},
			Capabilities:        &cb.Capabilities{Capabilities: map[string]*cb.Capability{capabilities.OrdererV2_0: {}}},
		},
		orgs: map[string]OrdererOrg{},
	}
	require.NoError(t, oc.Validate())

	require.Equal(t, "etcdraft", oc.ConsensusType())
	require.Equal(t, []byte("md"), oc.ConsensusMetadata())
	require.Equal(t, ab.ConsensusType_STATE_MAINTENANCE, oc.ConsensusState())
	require.Equal(t, &ab.BatchSize{MaxMessageCount: 10, AbsoluteMaxBytes: 1000, PreferredMaxBytes: 500}, oc.BatchSize())
	require.Equal(t, 2*time.Second, oc.BatchTimeout())
	require.Equal(t, uint64(7), oc.MaxChannelsCount())
	require.Empty(t, oc.Organizations())
	require.Equal(t, consenters, oc.Consenters())
	require.NoError(t, oc.Capabilities().Supported())
	require.True(t, oc.Capabilities().UseChannelCreationPolicyAsAdmins())
}

func TestOrdererOrgConfigEndpoints(t *testing.T) {
	t.Parallel()
	ooc := &OrdererOrgConfig{}
	require.Nil(t, ooc.Endpoints())
}

func TestNewOrdererOrgConfig(t *testing.T) {
	t.Parallel()

	t.Run("SubGroupError", func(t *testing.T) {
		t.Parallel()
		_, err := NewOrdererOrgConfig("org1", &cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"sub": {}}}, nil, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "does not allow sub-groups")
	})

	t.Run("EndpointsNotAllowedBelowV1_4_2", func(t *testing.T) {
		t.Parallel()
		orgGroup := &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				EndpointsKey: {Value: protoutil.MarshalOrPanic(EndpointsValue([]string{"o:7050"}).Value())},
			},
		}
		_, err := NewOrdererOrgConfig("org1", orgGroup, nil, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "cannot contain endpoints value")
	})

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		orgGroup := &cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}
		_, err := NewOrdererOrgConfig("org1", orgGroup, nil, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("NoMSPValue", func(t *testing.T) {
		t.Parallel()
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, cryptoProvider)
		_, err = NewOrdererOrgConfig("org1", &cb.ConfigGroup{}, mspConfigHandler, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "setting up the MSP manager failed")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		conf, err := msp.GetLocalMspConfig(getDevMspDir(), nil, "SampleOrg")
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())

		orgGroup := &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				MSPKey:       {Value: protoutil.MarshalOrPanic(MSPValue(conf).Value())},
				EndpointsKey: {Value: protoutil.MarshalOrPanic(EndpointsValue([]string{"o:7050"}).Value())},
			},
		}

		ooc, err := NewOrdererOrgConfig("org1", orgGroup, mspConfigHandler, capabilities.NewChannelProvider(map[string]*cb.Capability{capabilities.ChannelV3_0: {}}))
		require.NoError(t, err)
		require.Equal(t, "org1", ooc.Name())
		require.Equal(t, "SampleOrg", ooc.MSPID())
		require.NotNil(t, ooc.MSP())
		require.Equal(t, []string{"o:7050"}, ooc.Endpoints())
	})
}

func TestNewOrdererConfig(t *testing.T) {
	t.Parallel()

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		ordererGroup := &cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}
		_, err := NewOrdererConfig(ordererGroup, nil, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("EmptyGroupFailsValidate", func(t *testing.T) {
		t.Parallel()
		_, err := NewOrdererConfig(&cb.ConfigGroup{}, nil, capabilities.NewChannelProvider(nil))
		require.EqualError(t, err, "attempted to set the batch size max message count to an invalid value: 0")
	})

	baseValues := func() map[string]*cb.ConfigValue {
		return map[string]*cb.ConfigValue{
			BatchSizeKey:     {Value: protoutil.MarshalOrPanic(BatchSizeValue(10, 1000, 500).Value())},
			BatchTimeoutKey:  {Value: protoutil.MarshalOrPanic(BatchTimeoutValue("1s").Value())},
			ConsensusTypeKey: {Value: protoutil.MarshalOrPanic(ConsensusTypeValue("etcdraft", nil).Value())},
		}
	}

	t.Run("OrgSubGroupError", func(t *testing.T) {
		t.Parallel()
		ordererGroup := &cb.ConfigGroup{
			Values: baseValues(),
			Groups: map[string]*cb.ConfigGroup{
				"org1": {Groups: map[string]*cb.ConfigGroup{"nested": {}}},
			},
		}
		_, err := NewOrdererConfig(ordererGroup, nil, capabilities.NewChannelProvider(nil))
		require.ErrorContains(t, err, "does not allow sub-groups")
	})

	t.Run("BFTRequiresOrgEndpoints", func(t *testing.T) {
		t.Parallel()
		conf, err := msp.GetLocalMspConfig(getDevMspDir(), nil, "SampleOrg")
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())

		ordererGroup := &cb.ConfigGroup{
			Values: baseValues(),
			Groups: map[string]*cb.ConfigGroup{
				"org1": {Values: map[string]*cb.ConfigValue{
					MSPKey: {Value: protoutil.MarshalOrPanic(MSPValue(conf).Value())},
				}},
			},
		}
		_, err = NewOrdererConfig(ordererGroup, mspConfigHandler, capabilities.NewChannelProvider(map[string]*cb.Capability{capabilities.ChannelV3_0: {}}))
		require.ErrorContains(t, err, "some orderer organizations endpoints are empty")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		conf, err := msp.GetLocalMspConfig(getDevMspDir(), nil, "SampleOrg")
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())

		ordererGroup := &cb.ConfigGroup{
			Values: baseValues(),
			Groups: map[string]*cb.ConfigGroup{
				"org1": {Values: map[string]*cb.ConfigValue{
					MSPKey:       {Value: protoutil.MarshalOrPanic(MSPValue(conf).Value())},
					EndpointsKey: {Value: protoutil.MarshalOrPanic(EndpointsValue([]string{"o:7050"}).Value())},
				}},
			},
		}
		oc, err := NewOrdererConfig(ordererGroup, mspConfigHandler, capabilities.NewChannelProvider(map[string]*cb.Capability{capabilities.ChannelV3_0: {}}))
		require.NoError(t, err)
		require.Len(t, oc.Organizations(), 1)
		require.Equal(t, "etcdraft", oc.ConsensusType())
	})
}
