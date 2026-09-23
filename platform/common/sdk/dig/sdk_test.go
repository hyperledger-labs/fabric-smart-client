/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package dig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
)

// Compile-time checks ensuring BaseSDK satisfies dig.SDK and node.SDK.
var (
	_ dig.SDK  = (*dig.BaseSDK)(nil)
	_ node.SDK = (*dig.BaseSDK)(nil)
)

type mockContainer struct {
	dig.Container
}

type mockConfigService struct {
	driver.ConfigService
}

func TestBaseSDK(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		container  dig.Container
		configServ driver.ConfigService
	}{
		{
			name:       "non-nil dependencies",
			container:  &mockContainer{},
			configServ: &mockConfigService{},
		},
		{
			name:       "nil dependencies",
			container:  nil,
			configServ: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sdk := dig.NewBaseSDK(tc.container, tc.configServ)
			require.NotNil(t, sdk)

			// Accessor methods
			assert.Equal(t, tc.container, sdk.Container())
			assert.Equal(t, tc.configServ, sdk.ConfigService())

			// Lifecycle methods
			ctx := t.Context()
			assert.NoError(t, sdk.Install())
			assert.NoError(t, sdk.Start(ctx))
			assert.NoError(t, sdk.PostStart(ctx))
			assert.NoError(t, sdk.Stop())
		})
	}
}
