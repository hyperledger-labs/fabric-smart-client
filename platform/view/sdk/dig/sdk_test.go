/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
)

func TestWiring(t *testing.T) {
	t.Parallel()
	require.NoError(t, DryRunWiring(digutils.Identity[dig2.SDK]()))
}
