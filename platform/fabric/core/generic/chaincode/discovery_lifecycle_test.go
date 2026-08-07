/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mock"
)

// stableGoroutineCount waits for the number of live goroutines to settle and
// returns it, to avoid flaking on goroutines that are mid-teardown.
func stableGoroutineCount() int {
	var last int
	for range 10 {
		runtime.GC()
		time.Sleep(20 * time.Millisecond)
		n := runtime.NumGoroutine()
		if n == last {
			return n
		}
		last = n
	}
	return last
}

// A Manager's discovery cache goroutine must stop when the Manager is stopped.
//
// Deliberately not t.Parallel(): it counts live goroutines process-wide, so
// it must not race against other tests spawning/tearing down goroutines of
// their own.
func TestManager_Stop_StopsDiscoveryCache(t *testing.T) { //nolint:paralleltest // measures process-wide goroutine counts; must run serially
	before := stableGoroutineCount()

	ctx := t.Context()

	mockCS := &mock.ConfigService{}
	mockCS.NetworkNameReturns("test-network")
	mockCC := &mock.ChannelConfig{}
	mockCC.IDReturns("test-channel")
	mockCC.GetNumRetriesReturns(1)
	mockCC.GetRetrySleepReturns(0)
	mockCC.DiscoveryTimeoutReturns(time.Second)
	mockCC.DiscoveryDefaultTTLSReturns(time.Second)

	m := chaincode.NewManager(
		ctx,
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
	require.GreaterOrEqual(t, stableGoroutineCount()-before, 1)

	m.Stop()
	// <=1 (not <=0): the polling goroutine require.Eventually spawns to run
	// this very condition function is itself alive and counted while it runs.
	require.Eventually(t, func() bool {
		return stableGoroutineCount()-before <= 1
	}, 5*time.Second, 50*time.Millisecond)
}
