/*
Copyright IBM Corp. 2017 All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig/capabilities"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func TestApplicationInterface(t *testing.T) {
	t.Parallel()
	_ = Application((*ApplicationConfig)(nil))
}

func TestACL(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)
	cgt := &cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			ACLsKey: {
				Value: protoutil.MarshalOrPanic(
					ACLValues(map[string]string{}).Value(),
				),
			},
			CapabilitiesKey: {
				Value: protoutil.MarshalOrPanic(
					CapabilitiesValue(map[string]bool{
						capabilities.ApplicationV1_2: true,
					}).Value(),
				),
			},
		},
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		cg := proto.Clone(cgt).(*cb.ConfigGroup)
		_, err := NewApplicationConfig(proto.Clone(cg).(*cb.ConfigGroup), nil)
		g.Expect(err).NotTo(HaveOccurred())
	})
}

func TestApplicationConfigAPIPolicyMapper(t *testing.T) {
	t.Parallel()
	appGroup := &cb.ConfigGroup{
		Values: map[string]*cb.ConfigValue{
			ACLsKey: {
				Value: protoutil.MarshalOrPanic(
					ACLValues(map[string]string{"api": "/Channel/Application/Writers"}).Value(),
				),
			},
		},
	}
	ac, err := NewApplicationConfig(appGroup, nil)
	require.NoError(t, err)
	require.Equal(t, "/Channel/Application/Writers", ac.APIPolicyMapper().PolicyRefForAPI("api"))
	require.Empty(t, ac.APIPolicyMapper().PolicyRefForAPI("missing"))
}

func TestNewApplicationConfigErrors(t *testing.T) {
	t.Parallel()

	t.Run("UnknownValueKey", func(t *testing.T) {
		t.Parallel()
		_, err := NewApplicationConfig(&cb.ConfigGroup{Values: map[string]*cb.ConfigValue{"Bogus": {}}}, nil)
		require.ErrorContains(t, err, "unexpected key Bogus")
	})

	t.Run("OrgSubGroupError", func(t *testing.T) {
		t.Parallel()
		appGroup := &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"org1": {Groups: map[string]*cb.ConfigGroup{"nested": {}}},
			},
		}
		_, err := NewApplicationConfig(appGroup, nil)
		require.ErrorContains(t, err, "does not allow sub-groups")
	})
}
