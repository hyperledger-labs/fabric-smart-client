/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package channelconfig

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
)

func TestConsortiums(t *testing.T) {
	t.Parallel()
	cc, err := NewConsortiumsConfig(&cb.ConfigGroup{}, nil)
	require.NoError(t, err)
	require.Empty(t, cc.Consortiums())
}

func TestNewConsortiumsConfig(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		cc, err := NewConsortiumsConfig(&cb.ConfigGroup{Groups: map[string]*cb.ConfigGroup{"c1": {}}}, nil)
		require.NoError(t, err)
		require.Contains(t, cc.Consortiums(), "c1")
	})

	t.Run("ConsortiumGroupError", func(t *testing.T) {
		t.Parallel()
		consortiumsGroup := &cb.ConfigGroup{
			Groups: map[string]*cb.ConfigGroup{
				"c1": {Values: map[string]*cb.ConfigValue{"Bogus": {}}},
			},
		}
		_, err := NewConsortiumsConfig(consortiumsGroup, nil)
		require.ErrorContains(t, err, "unexpected key Bogus")
	})
}
