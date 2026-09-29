/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"math"
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig/capabilities"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestInterface(t *testing.T) {
	t.Parallel()
	_ = Channel(&ChannelConfig{})
}

func TestChannelConfig(t *testing.T) {
	t.Parallel()
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	cc, err := NewChannelConfig(
		&cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"UnknownGroupKey": {}}},
		cryptoProvider,
	)
	require.Error(t, err)
	require.Nil(t, cc)
}

func TestBlockDataHashingStructure(t *testing.T) {
	t.Parallel()
	cc := &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{}}}
	require.Error(t, cc.validateBlockDataHashingStructure(), "Must supply block data hashing structure")

	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: 7}}}
	require.Error(t, cc.validateBlockDataHashingStructure(), "Invalid Merkle tree width supplied")

	var width uint32 = math.MaxUint32
	cc = &ChannelConfig{protos: &ChannelProtos{BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: width}}}
	require.NoError(t, cc.validateBlockDataHashingStructure(), "Valid Merkle tree width supplied")

	require.Equal(t, width, cc.BlockDataHashingStructureWidth(), "Unexpected width returned")
}

func TestOrdererAddresses(t *testing.T) {
	t.Parallel()
	cc := &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{}}}
	require.Error(t, cc.validateOrdererAddresses(), "Must supply orderer addresses")

	cc = &ChannelConfig{protos: &ChannelProtos{OrdererAddresses: &cb.OrdererAddresses{Addresses: []string{"127.0.0.1:7050"}}}}
	require.NoError(t, cc.validateOrdererAddresses(), "Invalid orderer address supplied")

	require.Equal(t, "127.0.0.1:7050", cc.OrdererAddresses()[0], "Unexpected orderer address returned")
}

func TestConsortiumName(t *testing.T) {
	t.Parallel()
	cc := &ChannelConfig{protos: &ChannelProtos{Consortium: &cb.Consortium{Name: "TestConsortium"}}}
	require.Equal(t, "TestConsortium", cc.ConsortiumName(), "Unexpected consortium name returned")
}

func TestChannelConfigGetters(t *testing.T) {
	t.Parallel()
	mspManager := msp.NewMSPManager()
	ordererConfig := &OrdererConfig{}
	consortiumsConfig := &ConsortiumsConfig{}
	cc := &ChannelConfig{
		protos:            &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA256}},
		mspManager:        mspManager,
		ordererConfig:     ordererConfig,
		consortiumsConfig: consortiumsConfig,
	}

	require.Same(t, mspManager, cc.MSPManager())
	require.Same(t, ordererConfig, cc.OrdererConfig())
	require.Same(t, consortiumsConfig, cc.ConsortiumsConfig())

	require.NoError(t, cc.validateHashingAlgorithm())
	require.Len(t, cc.HashingAlgorithm()([]byte("x")), 32)
}

func TestValidateHashingAlgorithm(t *testing.T) {
	t.Parallel()
	cc := &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: bccsp.SHA3_256}}}
	require.NoError(t, cc.validateHashingAlgorithm())
	require.Len(t, cc.HashingAlgorithm()([]byte("x")), 32)

	cc = &ChannelConfig{protos: &ChannelProtos{HashingAlgorithm: &cb.HashingAlgorithm{Name: "MD5"}}}
	require.EqualError(t, cc.validateHashingAlgorithm(), "unknown hashing algorithm type: MD5")
}

func TestChannelConfigValidate(t *testing.T) {
	t.Parallel()
	validProtos := func() *ChannelProtos {
		return &ChannelProtos{
			HashingAlgorithm:          &cb.HashingAlgorithm{Name: bccsp.SHA256},
			BlockDataHashingStructure: &cb.BlockDataHashingStructure{Width: math.MaxUint32},
			OrdererAddresses:          &cb.OrdererAddresses{},
		}
	}

	t.Run("BadHashingAlgorithm", func(t *testing.T) {
		t.Parallel()
		protos := validProtos()
		protos.HashingAlgorithm.Name = "MD5"
		cc := &ChannelConfig{protos: protos}
		require.EqualError(t, cc.Validate(capabilities.NewChannelProvider(nil)), "unknown hashing algorithm type: MD5")
	})

	t.Run("BadWidth", func(t *testing.T) {
		t.Parallel()
		protos := validProtos()
		protos.BlockDataHashingStructure.Width = 7
		cc := &ChannelConfig{protos: protos}
		require.ErrorContains(t, cc.Validate(capabilities.NewChannelProvider(nil)), "BlockDataHashStructure width")
	})

	t.Run("V1_0RequiresOrdererAddresses", func(t *testing.T) {
		t.Parallel()
		cc := &ChannelConfig{protos: validProtos()}
		require.EqualError(t, cc.Validate(capabilities.NewChannelProvider(nil)), "must set some OrdererAddresses")
	})

	t.Run("V3_0ForbidsOrdererAddresses", func(t *testing.T) {
		t.Parallel()
		protos := validProtos()
		protos.OrdererAddresses.Addresses = []string{"a:1"}
		cc := &ChannelConfig{protos: protos}
		require.ErrorContains(t, cc.Validate(capabilities.NewChannelProvider(map[string]*cb.Capability{capabilities.ChannelV3_0: {}})), "global OrdererAddresses are not allowed")
	})

	t.Run("V3_0NoAddressesOK", func(t *testing.T) {
		t.Parallel()
		cc := &ChannelConfig{protos: validProtos()}
		require.NoError(t, cc.Validate(capabilities.NewChannelProvider(map[string]*cb.Capability{capabilities.ChannelV3_0: {}})))
	})
}

func TestNewChannelConfigErrors(t *testing.T) {
	t.Parallel()

	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		_, err := NewChannelConfig(&cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}, cryptoProvider)
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("EmptyGroupFailsValidate", func(t *testing.T) {
		t.Parallel()
		_, err := NewChannelConfig(&cb.ConfigGroup{}, cryptoProvider)
		require.EqualError(t, err, "unknown hashing algorithm type: ")
	})

	validValues := func() map[string]*cb.ConfigValue {
		return map[string]*cb.ConfigValue{
			HashingAlgorithmKey:          {Value: protoutil.MarshalOrPanic(HashingAlgorithmValue().Value())},
			BlockDataHashingStructureKey: {Value: protoutil.MarshalOrPanic(BlockDataHashingStructureValue().Value())},
			OrdererAddressesKey:          {Value: protoutil.MarshalOrPanic(OrdererAddressesValue([]string{"orderer:7050"}).Value())},
		}
	}

	t.Run("UnknownSubGroup", func(t *testing.T) {
		t.Parallel()
		_, err := NewChannelConfig(&cb.ConfigGroup{
			Values: validValues(),
			Groups: map[string]*cb.ConfigGroup{"Bogus": {}},
		}, cryptoProvider)
		require.ErrorContains(t, err, "disallowed channel group")
	})

	t.Run("OrdererSubGroupError", func(t *testing.T) {
		t.Parallel()
		_, err := NewChannelConfig(&cb.ConfigGroup{
			Values: validValues(),
			Groups: map[string]*cb.ConfigGroup{OrdererGroupKey: {}},
		}, cryptoProvider)
		require.ErrorContains(t, err, "could not create channel Orderer sub-group config")
	})

	t.Run("ConsortiumsSubGroupSuccess", func(t *testing.T) {
		t.Parallel()
		cc, err := NewChannelConfig(&cb.ConfigGroup{
			Values: validValues(),
			Groups: map[string]*cb.ConfigGroup{ConsortiumsGroupKey: {}},
		}, cryptoProvider)
		require.NoError(t, err)
		require.NotNil(t, cc.ConsortiumsConfig())
		require.NotNil(t, cc.MSPManager())
	})
}
