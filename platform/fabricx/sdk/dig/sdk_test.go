/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func TestWiring(t *testing.T) {
	t.Parallel()
	require.NoError(t, sdk.DryRunWiring(func(sdk common.SDK) *SDK { return NewFrom(fabric.NewFrom(sdk)) }, sdk.WithBool("fabric.enabled", true)))
}
