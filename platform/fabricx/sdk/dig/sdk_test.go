/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	fabric "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/stretchr/testify/require"
)

func TestWiring(t *testing.T) {
	require.NoError(t, sdk.DryRunWiring(func(sdk common.SDK) *SDK { return NewFrom(fabric.NewFrom(sdk)) }, sdk.WithBool("fabric.enabled", true)))
}
