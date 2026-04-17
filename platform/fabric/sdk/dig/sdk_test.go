/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
)

func TestWiring(t *testing.T) {
	t.Parallel()
	assert.NoError(t, sdk.DryRunWiring(NewFrom, sdk.WithBool("fabric.enabled", true)))
}
