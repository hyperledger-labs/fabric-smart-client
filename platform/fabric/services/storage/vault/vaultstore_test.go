/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
)

type vc int

const (
	_ vc = iota
	valid
	invalid
	busy
	unknown
)

// matrixItem describes one pagination walk over the eight transactions
// testPagination writes: the query each page renders to and the TxIDs it
// returns, in any order.
type matrixItem struct {
	pagination   driver.Pagination
	pages        [][]string
	sqlForward   []string
	argsForward  [][]sqlbuild.Param
	sqlBackward  []string
	argsBackward [][]sqlbuild.Param
	deserialize  func([]byte) (driver.Pagination, error)
}

func paginationMatrix(t *testing.T) []matrixItem {
	t.Helper()

	offset, err := pagination.Offset(0, 2)
	require.NoError(t, err)
	keyset, err := pagination.KeysetWithField[string](0, 2, "tx_id", "TxID")
	require.NoError(t, err)

	return []matrixItem{
		{
			pagination: pagination.None(),
			deserialize: func(data []byte) (driver.Pagination, error) {
				return pagination.NoneFromRaw(data)
			},
			sqlForward: []string{
				"SELECT * FROM test",
				"SELECT * FROM test LIMIT 0", // This makes sure that no rows are returned, so we get the same behaviour as for offset pagination
			},
			argsForward: [][]sqlbuild.Param{
				nil,
				nil,
			},
			pages: [][]string{
				{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025"},
			},
		},
		{
			pagination: offset,
			deserialize: func(data []byte) (driver.Pagination, error) {
				return pagination.OffsetFromRaw(data)
			},
			sqlForward: []string{
				"SELECT * FROM test LIMIT 2",
				"SELECT * FROM test LIMIT 2 OFFSET 2",
				"SELECT * FROM test LIMIT 2 OFFSET 4",
				"SELECT * FROM test LIMIT 2 OFFSET 6",
				"SELECT * FROM test LIMIT 2 OFFSET 8",
			},
			argsForward: [][]sqlbuild.Param{nil, nil, nil, nil, nil},
			pages: [][]string{
				{"txid1", "txid2"},
				{"txid10", "txid12"},
				{"txid21", "txid100"},
				{"txid200", "txid1025"},
			},
		},
		{
			pagination: keyset,
			deserialize: func(data []byte) (driver.Pagination, error) {
				return pagination.KeysetFromRaw[string](data, "TxID")
			},
			sqlForward: []string{
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2",
				"SELECT * FROM test WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 2",
				"SELECT * FROM test WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 2",
				"SELECT * FROM test WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 2",
				"SELECT * FROM test WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 2",
			},
			argsForward: [][]sqlbuild.Param{
				nil,
				{"txid10"},
				{"txid1025"},
				{"txid2"},
				{"txid21"},
			},

			sqlBackward: []string{
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2",
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2 OFFSET 2",
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2 OFFSET 4",
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2 OFFSET 6",
				"SELECT * FROM test ORDER BY tx_id ASC LIMIT 2 OFFSET 8",
			},
			argsBackward: [][]sqlbuild.Param{nil, nil, nil, nil, nil},

			pages: [][]string{
				{"txid1", "txid10"},
				{"txid100", "txid1025"},
				{"txid12", "txid2"},
				{"txid200", "txid21"},
			},
		},
	}
}

// renderPaginated builds the query the store would build for this pagination.
// The vault store does not expose its SQL, so this mirrors the shape produced
// by vaultReader.queryStatus and asserts on it directly.
func renderPaginated(pag driver.Pagination) (string, []sqlbuild.Param) {
	p := pagination.Paging(pag)
	return sqlbuild.New().
		WriteString("SELECT * FROM test").
		WriteWhere(p.Where).
		WritePaging(p).
		Build()
}

// readPage returns the TxIDs of one page of GetAllTxStatuses along with the
// pagination the store hands back for it.
func readPage(t *testing.T, store driver.VaultStore, pag driver.Pagination) ([]string, driver.Pagination) {
	t.Helper()
	p, err := store.GetAllTxStatuses(context.Background(), pag)
	require.NoError(t, err)
	statuses, err := collections.ReadAll(p.Items)
	require.NoError(t, err)
	txIDs := make([]string, len(statuses))
	for i, s := range statuses {
		txIDs[i] = s.TxID
	}
	return txIDs, p.Pagination
}

func testPagination(t *testing.T, store driver.VaultStore) {
	t.Helper()

	err := store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "",
		"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025")
	require.NoError(t, err)

	for _, item := range paginationMatrix(t) {
		if len(item.sqlBackward) == 0 {
			item.sqlBackward = item.sqlForward
			item.argsBackward = item.argsForward
		}
		pag := item.pagination
		page := 0
		for ; true; page++ {
			// We don't need to build the query here but we do it just to test that it was build correctly
			query, args := renderPaginated(pag)
			require.Equal(t, item.sqlForward[page], query)
			require.Equal(t, item.argsForward[page], args)

			txIDs, returned := readPage(t, store, pag)
			// Test we get 0 statuses when we reach the end
			if len(txIDs) == 0 {
				break
			}
			require.Less(t, page, len(item.pages))
			require.ElementsMatch(t, item.pages[page], txIDs)

			// the pagination survives a serialization round trip
			buf, err := returned.Serialize()
			require.NoError(t, err)
			returned, err = item.deserialize(buf)
			require.NoError(t, err)

			pag, err = returned.Next()
			require.NoError(t, err)
		}
		// test that we read everything
		require.Len(t, item.pages, page)

		// Test that Prev() work. Start by advancing the pagination pointer 2 steps forward
		if len(item.pages) < 2 {
			continue
		}
		pag = item.pagination
		for page = 0; page < 2; page++ {
			pag, err = pag.Next()
			require.NoError(t, err)
		}
		for ; page >= 0; page-- {
			query, _ := renderPaginated(pag)
			require.Equal(t, item.sqlBackward[page], query)
			var txIDs []string
			txIDs, pag = readPage(t, store, pag)
			require.ElementsMatch(t, item.pages[page], txIDs)
			if page > 0 {
				pag, err = pag.Prev()
				require.NoError(t, err)
			}
		}
	}
}

// The memory driver keeps one SQLite database per process, so each memory test
// opens its tables under a name no other test uses.
func TestPaginationStoreMem(t *testing.T) {
	t.Parallel()
	db, err := OpenMemoryVault("paginationdb")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() {
		_ = db.Close()
	})

	testPagination(t, db)
}

func TestPaginationStoreSqlite(t *testing.T) {
	t.Parallel()
	db, err := OpenSqliteVault("testdb", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() {
		_ = db.Close()
	})

	testPagination(t, db)
}

func TestPaginationStoreSPostgres(t *testing.T) {
	t.Parallel()
	db, terminate, err := OpenPostgresVault("testdb")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(terminate)

	testPagination(t, db)
}

func TestVaultStoreMem(t *testing.T) {
	t.Parallel()
	db, err := OpenMemoryVault("vaultstoredb")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() {
		_ = db.Close()
	})

	testVaultStore(t, db)
	testOneMore(t, db)
}

func TestVaultStoreSqlite(t *testing.T) {
	t.Parallel()
	db, err := OpenSqliteVault("testdb", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() {
		_ = db.Close()
	})

	testVaultStore(t, db)
	testOneMore(t, db)
}

func TestVaultStorePostgres(t *testing.T) {
	t.Parallel()
	db, terminate, err := OpenPostgresVault("testdb")
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(terminate)

	testVaultStore(t, db)
	testOneMore(t, db)
}

func testOneMore(t *testing.T, store driver.VaultStore) {
	t.Helper()

	err := store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "", "txid3")
	require.NoError(t, err)

	tx, err := store.GetTxStatus(context.Background(), "txid3")
	require.NoError(t, err)
	require.Equal(t, driver.TxStatusCode(valid), tx.Code)

	txids, _ := readPage(t, store, pagination.None())
	require.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3"}, txids)

	last, err := store.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "txid3", last.TxID)

	// add a busy tx
	err = store.SetStatuses(context.Background(), driver.TxStatusCode(busy), "", "txid4")
	require.NoError(t, err)

	last, err = store.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "txid3", last.TxID)

	// iterate again
	txids, _ = readPage(t, store, pagination.None())
	require.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4"}, txids)

	// update the busy tx
	err = store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "", "txid4")
	require.NoError(t, err)

	last, err = store.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "txid4", last.TxID)

	// iterate again
	txids, _ = readPage(t, store, pagination.None())
	require.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025", "txid3", "txid4"}, txids)
}

func testVaultStore(t *testing.T, store driver.VaultStore) {
	t.Helper()

	txids, _ := readPage(t, store, pagination.None())
	require.Empty(t, txids)

	err := store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "",
		"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025")
	require.NoError(t, err)

	itr, err := store.GetTxStatuses(context.Background(), "txid3", "txid10")
	require.NoError(t, err)
	txStatuses, err := collections.ReadAll(itr)
	require.NoError(t, err)

	// Build lookup
	statusByID := make(map[string]driver.TxStatusCode)
	for _, s := range txStatuses {
		statusByID[s.TxID] = s.Code
	}
	require.NotContains(t, statusByID, "txid3")
	require.Contains(t, statusByID, "txid10")
	require.Equal(t, driver.TxStatusCode(valid), statusByID["txid10"])

	last, err := store.GetLast(context.Background())
	require.NoError(t, err)
	require.Equal(t, "txid1025", last.TxID)

	txids, _ = readPage(t, store, pagination.None())
	require.Equal(t, []string{"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025"}, txids)

	itr, err = store.GetTxStatuses(context.Background(), "boh")
	require.NoError(t, err)
	txStatuses, err = collections.ReadAll(itr)
	require.NoError(t, err)
	require.Empty(t, txStatuses)

	itr, err = store.GetTxStatuses(context.Background(), "txid1025", "txid999", "txid21")
	require.NoError(t, err)
	txStatuses, err = collections.ReadAll(itr)
	require.NoError(t, err)
	txids = make([]driver.TxID, len(txStatuses))
	for i, txStatus := range txStatuses {
		txids[i] = txStatus.TxID
	}
	require.Contains(t, txids, "txid21")
	require.Contains(t, txids, "txid1025")
}
