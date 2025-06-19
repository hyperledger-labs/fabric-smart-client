/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/stretchr/testify/assert"
)

func TestWiring(t *testing.T) {
	assert.NoError(t, DryRunWiring(digutils.Identity[dig2.SDK]()))
}
