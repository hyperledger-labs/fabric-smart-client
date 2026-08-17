/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mock"
)

// TestManager_Stop_StopsDiscoveryCache asserts a Manager's per-chaincode discovery
// cache goroutine stops when the Manager is stopped.
//
// The Manager is rooted at a context this test never cancels, so Stop() is the only
// thing that can retire the cache goroutine; goleak fails the test if it survives.
func TestManager_Stop_StopsDiscoveryCache(t *testing.T) { //nolint:paralleltest // uses goleak.VerifyNone; must run serially
	defer goleak.VerifyNone(t)

	mockCS := &mock.ConfigService{}
	mockCS.NetworkNameReturns("test-network")
	mockCC := &mock.ChannelConfig{}
	mockCC.IDReturns("test-channel")
	mockCC.GetNumRetriesReturns(1)
	mockCC.GetRetrySleepReturns(0)
	mockCC.DiscoveryTimeoutReturns(time.Second)
	mockCC.DiscoveryDefaultTTLSReturns(time.Second)

	m := chaincode.NewManager(
		context.Background(),
		"test-network",
		"test-channel",
		mockCS,
		mockCC,
		1,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	// Force creation of a chaincode (and thus its discovery cache).
	require.NotNil(t, m.Chaincode("cc"))

	m.Stop()
}
