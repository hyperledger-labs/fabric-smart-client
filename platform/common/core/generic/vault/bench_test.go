/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	blockSize = 100
	rwSetSize = 5
	namespace = "ns"
)

func BenchmarkPostgresVault(b *testing.B) {
	ddb, terminate, err := vault.OpenPostgresVault("common-sdk-node1")
	require.NoError(b, err)
	require.NotNil(b, ddb)
	defer terminate()

	runBenchmarkSetStateCommit(b, ddb)
}

func BenchmarkMemoryVault(b *testing.B) {
	ddb, err := vault.OpenMemoryVault()
	require.NoError(b, err)
	require.NotNil(b, ddb)

	runBenchmarkSetStateCommit(b, ddb)
}

func BenchmarkSqliteVault(b *testing.B) {
	ddb, err := vault.OpenSqliteVault("node1", b.TempDir())
	require.NoError(b, err)
	require.NotNil(b, ddb)

	runBenchmarkSetStateCommit(b, ddb)
}

func runBenchmarkSetStateCommit(b *testing.B, ddb driver.VaultStore) {
	b.Helper()
	vault, err := (&testArtifactProvider{}).NewNonCachedVault(ddb)
	require.NoError(b, err)
	defer func() { require.NoError(b, ddb.Close()) }()

	txs := make(chan *txCommitIndex, 1000)
	wg := runCommitter(b, txs, vault)

	b.StartTimer()
	ctr := atomic.Uint64{}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			txCtr := ctr.Add(1)
			commitIndex := &txCommitIndex{
				ctx:         context.Background(),
				txID:        fmt.Sprintf("tx%d", txCtr),
				block:       txCtr / blockSize,
				indexInBloc: txCtr % blockSize,
			}
			err := newRWSet(vault, commitIndex)
			require.NoError(b, err)
			txs <- commitIndex
		}
	})
	txs <- nil
	wg.Wait()
	b.StopTimer()

	verifyResult(b, vault)
}

func verifyResult(b *testing.B, vault *Vault[ValidationCode]) {
	b.Helper()
	stateItr, err := vault.vaultStore.GetAllStates(context.Background(), namespace)
	require.NoError(b, err)
	states, err := collections.ReadAll(stateItr)
	require.NoError(b, err)
	require.Len(b, states, b.N*rwSetSize, "all states are stored")
}

func newRWSet(vault *Vault[ValidationCode], commitIndex *txCommitIndex) error {
	rws, err := vault.NewRWSet(commitIndex.ctx, commitIndex.txID)
	if err != nil {
		return err
	}
	defer rws.Done()
	for i := 0; i < rwSetSize; i++ {
		key := fmt.Sprintf("%s_%d", commitIndex.txID, i)
		err = rws.SetState(namespace, key, []byte(key))
		if err != nil {
			return err
		}
	}
	return nil
}

func runCommitter(b *testing.B, txs chan *txCommitIndex, vault *Vault[ValidationCode]) *sync.WaitGroup {
	b.Helper()
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for commitIndex := range txs {
			if commitIndex == nil {
				return
			}
			err := vault.CommitTX(commitIndex.ctx, commitIndex.txID, commitIndex.block, commitIndex.indexInBloc)
			assert.NoError(b, err)
		}
	}()
	return wg
}
