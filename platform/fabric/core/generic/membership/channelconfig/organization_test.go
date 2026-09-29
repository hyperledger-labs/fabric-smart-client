/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/hyperledger/fabric-lib-go/bccsp/sw"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestOrganization(t *testing.T) {
	t.Parallel()
	_ = Org(&OrganizationConfig{})
}

func TestNewOrganizationConfig(t *testing.T) {
	t.Parallel()

	t.Run("SubGroupError", func(t *testing.T) {
		t.Parallel()
		_, err := NewOrganizationConfig("org1", &cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"sub": {}}}, nil)
		require.ErrorContains(t, err, "do not support sub-groups")
	})

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		_, err := NewOrganizationConfig("org1", &cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}, nil)
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("NoMSPValue", func(t *testing.T) {
		t.Parallel()
		cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, cryptoProvider)
		_, err = NewOrganizationConfig("org1", &cb.ConfigGroup{}, mspConfigHandler)
		require.ErrorContains(t, err, "setting up the MSP manager failed")
	})

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		conf, err := msp.GetLocalMspConfig(getDevMspDir(), nil, "SampleOrg")
		require.NoError(t, err)
		mspConfigHandler := NewMSPConfigHandler(msp.MSPv1_0, factory.GetDefault())

		orgGroup := &cb.ConfigGroup{
			Values: map[string]*cb.ConfigValue{
				MSPKey: {Value: protoutil.MarshalOrPanic(MSPValue(conf).Value())},
			},
		}
		oc, err := NewOrganizationConfig("org1", orgGroup, mspConfigHandler)
		require.NoError(t, err)
		require.Equal(t, "org1", oc.Name())
		require.Equal(t, "SampleOrg", oc.MSPID())
		require.NotNil(t, oc.MSP())
	})
}
