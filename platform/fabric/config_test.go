/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mock"
)

func TestConfigService(t *testing.T) {
	t.Parallel()

	mockConf := &mock.ConfigService{}
	mockConf.DriverNameReturns("mydriver")
	mockConf.GetStringReturns("myvalue")
	mockConf.DefaultChannelReturns("mychannel")

	// The ConfigService struct fields are unexported and we don't have a NewConfigService function.
	// Since we are in the fabric package, we can instantiate it directly.
	cs := &ConfigService{confService: mockConf}

	require.Equal(t, "mydriver", cs.DriverName())
	require.Equal(t, 1, mockConf.DriverNameCallCount())

	require.Equal(t, "myvalue", cs.GetString("mykey"))
	require.Equal(t, 1, mockConf.GetStringCallCount())
	require.Equal(t, "mykey", mockConf.GetStringArgsForCall(0))

	require.Equal(t, "mychannel", cs.DefaultChannel())
	require.Equal(t, 1, mockConf.DefaultChannelCallCount())
}
