/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func TestWiring(t *testing.T) {
	assert.NoError(sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", true)))
}
