/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

func countProvisionGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "idemix.(*IdentityCache).provisionIdentities")
}

func fetchIdemixIdentity(t *testing.T, s *service) {
	t.Helper()
	s.mspsMutex.RLock()
	loaded := s.mspsByName["idemix"]
	s.mspsMutex.RUnlock()
	require.NotNil(t, loaded)
	_, _, err := loaded.GetIdentity(nil)
	require.NoError(t, err)
}

// TestRefreshClosesIdemixIdentityCache loads an idemix MSP through IdentityLoader,
// the production path that used to discard the *IdentityCache, then refreshes.
// The provisioning goroutine from the previous load must stop, so a reload does
// not leave a second one running.
func TestRefreshClosesIdemixIdentityCache(t *testing.T) { //nolint:paralleltest // counts process-wide goroutines
	require.Equal(t, 0, countProvisionGoroutines())

	store, err := mem.NewDriver().NewKVS("")
	require.NoError(t, err)
	kvss, err := kvs.New(store, "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	signerInfo, err := mem.NewDriver().NewSignerInfo("")
	require.NoError(t, err)
	auditInfo, err := mem.NewDriver().NewAuditInfo("")
	require.NoError(t, err)
	sigService := sig.NewService(sig.NewMultiplexDeserializer(), auditInfo, signerInfo)

	s := NewLocalMSPManager(
		&testConfig{
			networkName: "testnet",
			defaultMSP:  "idemix",
			msps: []config.MSP{{
				ID:        "idemix",
				MSPType:   idemix.MSPType,
				MSPID:     "idemix",
				Path:      "idemix/testdata/idemix",
				CacheSize: 2,
			}},
		},
		kvss,
		nil,
		nil,
		nil,
		sig.NewMultiplexDeserializer(),
		2,
	)
	s.PutIdentityLoader(idemix.MSPType, &idemix.IdentityLoader{
		KVS:           kvss,
		SignerService: sigService,
	})

	require.ErrorContains(t, s.Load(), "no default identity")
	fetchIdemixIdentity(t, s)
	require.Eventually(t, func() bool {
		return countProvisionGoroutines() == 1
	}, 15*time.Second, 20*time.Millisecond)

	require.ErrorContains(t, s.Refresh(), "no default identity")
	require.Eventually(t, func() bool {
		return countProvisionGoroutines() == 0
	}, 15*time.Second, 20*time.Millisecond)

	fetchIdemixIdentity(t, s)
	require.Eventually(t, func() bool {
		return countProvisionGoroutines() == 1
	}, 15*time.Second, 20*time.Millisecond)

	require.ErrorContains(t, s.Refresh(), "no default identity")
	fetchIdemixIdentity(t, s)
	require.Eventually(t, func() bool {
		return countProvisionGoroutines() == 1
	}, 15*time.Second, 20*time.Millisecond, "a reload must not leave the previous provisioning goroutine running")
}
