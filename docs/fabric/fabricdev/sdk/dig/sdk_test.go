/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"testing"

	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
)

func TestWiring(t *testing.T) {
	assert.NoError(sdk.DryRunWiring(context.Background(), NewFrom, sdk.WithBool("fabric.enabled", true)))
}
