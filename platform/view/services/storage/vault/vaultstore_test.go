/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/stretchr/testify/assert"
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

type matrixItem struct {
	pagination driver.Pagination
	matcher    []types.GomegaMatcher
}

var matrix = []matrixItem{
	{
		pagination: common.NewNoPagination(),
		matcher: []types.GomegaMatcher{
			ConsistOf(
				HaveField("TxID", Equal("txid1")),
				HaveField("TxID", Equal("txid2")),
				HaveField("TxID", Equal("txid10")),
				HaveField("TxID", Equal("txid12")),
				HaveField("TxID", Equal("txid21")),
				HaveField("TxID", Equal("txid100")),
				HaveField("TxID", Equal("txid200")),
				HaveField("TxID", Equal("txid1025")),
			),
		},
	},
	{
		pagination: NewOffsetPagination(0, 2),
		matcher: []types.GomegaMatcher{
			ConsistOf(
				HaveField("TxID", Equal("txid1")),
				HaveField("TxID", Equal("txid2")),
			),
			ConsistOf(
				HaveField("TxID", Equal("txid10")),
				HaveField("TxID", Equal("txid12")),
			),
			ConsistOf(
				HaveField("TxID", Equal("txid21")),
				HaveField("TxID", Equal("txid100")),
			),
			ConsistOf(
				HaveField("TxID", Equal("txid200")),
				HaveField("TxID", Equal("txid1025")),
			),
		},
	},
}

func NewOffsetPagination(offset int, pageSize int) *common.OffsetPagination {
	offsetPagination, err := common.NewOffsetPagination(offset, pageSize)
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}
	return offsetPagination
}

func getAllTxStatuses(store driver.VaultStore, pagination driver.Pagination) ([]driver.TxStatus, error) {
	p, err := store.GetAllTxStatuses(context.Background(), pagination)
	if err != nil {
		return nil, err
	}
	statuses, err := collections.ReadAll(p.Items)
	if err != nil {
		return nil, err
	}
	return statuses, nil
}

func testPagination(t *testing.T, store driver.VaultStore) {
	RegisterFailHandler(Fail)
	err := store.SetStatuses(context.Background(), driver.TxStatusCode(valid), "",
		"txid1", "txid2", "txid10", "txid12", "txid21", "txid100", "txid200", "txid1025")
	Expect(err).ToNot(HaveOccurred())

	for _, item := range matrix {
		for page := 0; page < len(item.matcher); page++ {
			statuses, err := getAllTxStatuses(store, item.pagination)
			Expect(err).ToNot(HaveOccurred())
			if len(statuses) == 0 {
				break
			}
			Expect(statuses).To(item.matcher[page])
			item.pagination, err = item.pagination.Next()
			Expect(err).ToNot(HaveOccurred())
		}
		for page := len(item.matcher) - 1; 0 <= page; page-- {
			item.pagination, err = item.pagination.Prev()
			Expect(err).ToNot(HaveOccurred())
			statuses, err := getAllTxStatuses(store, item.pagination)
			Expect(err).ToNot(HaveOccurred())
			if len(statuses) == 0 {
				break
			}
			Expect(statuses).To(item.matcher[page])
		}

		item.pagination, err = item.pagination.Prev()
		Expect(err).ToNot(HaveOccurred())
		statuses, err := getAllTxStatuses(store, item.pagination)
		Expect(err).ToNot(HaveOccurred())
		if len(statuses) == 0 {
			break
		}
	}
}

func TestPaginationStoreMem(t *testing.T) {
	db, err := OpenMemoryVault(utils.GenerateUUIDOnlyLetters())
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testPagination(t, db)
}

func TestPaginationStoreSqlite(t *testing.T) {
	db, err := OpenSqliteVault("testdb", t.TempDir())
	assert.NoError(t, err)
	assert.NotNil(t, db)

	testPagination(t, db)
}

func TestPaginationStoreSPostgres(t *testing.T) {
	db, terminate, err := OpenPostgresVault("testdb")
	assert.NoError(t, err)
	assert.NotNil(t, db)
	defer terminate()

	testPagination(t, db)
}

func TestVaultStoreMem(t *testing.T) {
	db, err := OpenMemoryVault(utils.GenerateUUIDOnlyLetters())
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
	pageIt, err := store.GetAllTxStatuses(context.Background(), common.NewNoPagination())
	if err != nil {
		return nil, err
	}
	txStatuses, err := collections.ReadAll(pageIt.Items)
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
