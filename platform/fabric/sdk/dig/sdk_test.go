/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/stretchr/testify/assert"
)

func TestWiring(t *testing.T) {
	assert.NoError(t, sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", true)))
}
