/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"

	"github.com/test-go/testify/assert"
	"golang.org/x/exp/slices"
)

type vc int

const (
	_ vc = iota
	valid
	invalid
	busy
	unknown
)

func TestVaultStoreMem(t *testing.T) {
	db, err := OpenMemoryVault()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testVaultStore(t, db)
	testOneMore(t, db)
}

func TestVaultStoreSqlite(t *testing.T) {
	db, err := OpenSqliteVault("testdb", t.TempDir())
	assert.NoError(t, err)
	assert.NotNil(t, db)

	assert.NotNil(t, db)
	testVaultStore(t, db)
	testOneMore(t, db)
}

func TestVaultStorePostgres(t *testing.T) {
	db, terminate, err := OpenPostgresVault("testdb")
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer terminate()

	testVaultStore(t, db)
	testOneMore(t, db)
}

func testOneMore(t *testing.T, store driver.VaultStore) {
	err := store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "", "txid3")
	assert.NoError(t, err)

	tx, err := store.GetTxStatus(context.Background(), "txid3")
	assert.NoError(t, err)
	assert.Equal(t, driver.TxStatusCode(valid), tx.Code)

	txids, err := fetchAll(store)
	assert.NoError(t, err)
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3"}, txids)

	last, err := store.GetLast(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "txid3", last.TxID)

	// add a busy tx
	err = store.SetStatuses(context.Background(), driver.TxStatusCode(busy), "", "txid4")
	assert.NoError(t, err)

	last, err = store.GetLast(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "txid3", last.TxID)

	// iterate again
	txids, err = fetchAll(store)
	assert.NoError(t, err)
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4"}, txids)

	// update the busy tx
	err = store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "", "txid4")
	assert.NoError(t, err)

	last, err = store.GetLast(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "txid4", last.TxID)

	// iterate again
	txids, err = fetchAll(store)
	assert.NoError(t, err)
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4"}, txids)
}

func fetchAll(store driver.VaultStore) ([]driver.TxID, error) {

	p, err := store.GetAllTxStatuses(context.Background(), &common.OffsetPagination{Offset: 0, PageSize: 10})

	p, err = store.GetAllTxStatuses(context.Background(), p.Pagination.Next())

	// Loop whil p.next
	//     it = p.next

	// p, err := store.GetAllTxStatuses(context.Background(), p.Pagintion.Next())

	// itieate on p.it

	// P.GentNext
	if err != nil {
		return nil, err
	}
	txStatuses, err := collections.ReadAll(p.Items)
	if err != nil {
		return nil, err
	}
	txids := make([]driver.TxID, len(txStatuses))
	for i, txStatus := range txStatuses {
		txids[i] = txStatus.TxID
	}
	return txids, nil
}

func testVaultStore(t *testing.T, store driver.VaultStore) {
	txids, err := fetchAll(store)
	assert.NoError(t, err)
	assert.Empty(t, txids)

	err = store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "",
		"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025")
	assert.NoError(t, err)

	itr, err := store.GetTxStatuses(context.Background(), "txid3", "txid10")
	assert.NoError(t, err)
	txStatuses, err := collections.ReadAll(itr)
	assert.NoError(t, err)

	slices.ContainsFunc(txStatuses, func(s driver.TxStatus) bool { return s.TxID == "txid3" && s.Code == driver.TxStatusCode(unknown) })
	slices.ContainsFunc(txStatuses, func(s driver.TxStatus) bool { return s.TxID == "txid10" && s.Code == driver.TxStatusCode(valid) })

	last, err := store.GetLast(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "txid1025", last.TxID)

	txids, err = fetchAll(store)
	assert.NoError(t, err)
	assert.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025"}, txids)

	itr, err = store.GetTxStatuses(context.Background(), "boh")
	assert.NoError(t, err)
	txStatuses, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	assert.Empty(t, txStatuses)

	itr, err = store.GetTxStatuses(context.Background(), "txid1025", "txid999", "txid21")
	assert.NoError(t, err)
	txStatuses, err = collections.ReadAll(itr)
	assert.NoError(t, err)
	txids = make([]driver.TxID, len(txStatuses))
	for i, txStatus := range txStatuses {
		txids[i] = txStatus.TxID
	}
	assert.Contains(t, txids, "txid21")
	assert.Contains(t, txids, "txid1025")

}
