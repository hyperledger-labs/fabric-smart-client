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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/stretchr/testify/assert"
)

const blockSize = 100
const rwSetSize = 5
const namespace = "ns"

func BenchmarkPostgresVault(b *testing.B) {
	ddb, terminate, err := vault.OpenPostgresVault("common-sdk-node1")
	assert.NoError(b, err)
	assert.NotNil(b, ddb)
	defer terminate()

	BenchTestSetStateCommit(b, ddb)
}

func BenchmarkMemoryVault(b *testing.B) {
	ddb, err := vault.OpenMemoryVault()
	assert.NoError(b, err)
	assert.NotNil(b, ddb)

	BenchTestSetStateCommit(b, ddb)
}

func BenchmarkSqliteVault(b *testing.B) {
	ddb, err := vault.OpenSqliteVault("node1", b.TempDir())
	assert.NoError(b, err)
	assert.NotNil(b, ddb)

	BenchTestSetStateCommit(b, ddb)
}

func BenchTestSetStateCommit(b *testing.B, ddb driver.VaultStore) {
	vault, err := (&testArtifactProvider{}).NewNonCachedVault(ddb)
	assert.NoError(b, err)
	defer func() { assert.NoError(b, ddb.Close()) }()

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
			assert.NoError(b, err)
			txs <- commitIndex
		}
	})
	txs <- nil
	wg.Wait()
	b.StopTimer()

	verifyResult(b, vault)
}

func verifyResult(b *testing.B, vault *Vault[ValidationCode]) {
	stateItr, err := vault.vaultStore.GetAllStates(context.Background(), namespace)
	assert.NoError(b, err)
	states, err := collections.ReadAll(stateItr)
	assert.NoError(b, err)
	assert.Len(b, states, b.N*rwSetSize, "all states are stored")
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
