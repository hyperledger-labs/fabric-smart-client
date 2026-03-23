/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	fscconfig "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointsLenCheck(t *testing.T) {
	fscConfigProvider, err := fscconfig.NewProvider("./testdata")
	require.NoError(t, err)

	p, err := config.NewProvider(fscConfigProvider)
	require.NoError(t, err)

	configService, err := p.GetConfig("default")
	require.NoError(t, err)

	c, err := finality.NewConfig(configService)
	require.NoError(t, err)

	assert.Len(t, c.Endpoints, 1)
}
