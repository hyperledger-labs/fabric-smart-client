/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	dbsql "database/sql"
	sqldriver "database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
)

// identity sanitizer: encodes and decodes to itself, so the asserted args are
// the raw values.
type idSanitizer struct{}

func (idSanitizer) Encode(s string) (string, error) { return s, nil }
func (idSanitizer) Decode(s string) (string, error) { return s, nil }

// isoMapper maps every isolation level to the default one. Only
// NewTxLockVaultReader's lazy reader would use it, and these tests never
// force that reader open.
type isoMapper struct{}

func (isoMapper) Map(driver.IsolationLevel) (dbsql.IsolationLevel, error) {
	return dbsql.LevelDefault, nil
}

// wrapError passes errors through unchanged.
type passthroughWrapper struct{}

func (passthroughWrapper) WrapError(err error) error { return err }

var vaultTables = common2.VaultTables{StateTable: "state", StatusTable: "status"}

func newVaultMock(t *testing.T) (*common2.VaultStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	Expect(err).ToNot(HaveOccurred())
	store := common2.NewVaultStore(db, db, vaultTables, passthroughWrapper{}, idSanitizer{}, isoMapper{})
	return store, mockDB
}

func TestVaultSQL_NewTxLockVaultReader(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectExec("INSERT INTO status (tx_id,code) VALUES ($1,$2) ON CONFLICT DO NOTHING").
		WithArgs("tx1", int64(driver.Busy)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
	Expect(err).ToNot(HaveOccurred())
	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
}

func TestVaultSQL_SetStatuses(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectExec("INSERT INTO status (tx_id,code,message) VALUES ($1,$2,$3),($4,$5,$6) "+
			"ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code, message = EXCLUDED.message").
		WithArgs("tx1", int64(driver.Valid), "msg", "tx2", int64(driver.Valid), "msg").
		WillReturnResult(sqlmock.NewResult(1, 2))

	Expect(store.SetStatuses(context.Background(), driver.Valid, "msg", "tx1", "tx2")).To(Succeed())
	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
}

func TestVaultSQL_SetStatusesBusy(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, _ := newVaultMock(t)

	query, args, err := store.SetStatusesBusy([]driver.TxID{"tx1", "tx2"})
	Expect(err).ToNot(HaveOccurred())
	Expect(query).To(Equal("INSERT INTO status (tx_id,code) VALUES ($1,$2),($3,$4) " +
		"ON CONFLICT (tx_id) DO UPDATE SET code = EXCLUDED.code"))
	Expect(args).To(Equal([]any{"tx1", driver.Busy, "tx2", driver.Busy}))
}

func TestVaultSQL_UpsertStates(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, _ := newVaultMock(t)

	// exactly one write, so map iteration order cannot make this flaky
	writes := driver.Writes{
		"ns": {"k1": driver.VaultValue{Raw: []byte("v"), Version: []byte("1")}},
	}
	query, args, err := store.UpsertStates(writes, driver.MetaWrites{})
	Expect(err).ToNot(HaveOccurred())
	Expect(query).To(Equal("INSERT INTO state (ns,pkey,val,kversion,metadata) VALUES ($1,$2,$3,$4,$5) " +
		"ON CONFLICT (ns,pkey) DO UPDATE SET val = EXCLUDED.val, kversion = EXCLUDED.kversion, metadata = EXCLUDED.metadata"))
	Expect(args).To(Equal([]any{"ns", "k1", []byte("v"), []byte("1"), []byte{}}))
}

func TestVaultSQL_SetStatusesValid(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, _ := newVaultMock(t)

	query, args, err := store.SetStatusesValid([]driver.TxID{"tx1", "tx2"})
	Expect(err).ToNot(HaveOccurred())
	Expect(query).To(Equal("UPDATE status SET code = $1 WHERE tx_id IN ($2,$3)"))
	Expect(args).To(Equal([]any{driver.Valid, "tx1", "tx2"}))

	query, args, err = store.SetStatusesValid([]driver.TxID{"tx1"})
	Expect(err).ToNot(HaveOccurred())
	Expect(query).To(Equal("UPDATE status SET code = $1 WHERE tx_id IN ($2)"))
	Expect(args).To(Equal([]any{driver.Valid, "tx1"}))
}

func TestVaultSQL_QueryState(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	stateRows := func(mockDB sqlmock.Sqlmock) *sqlmock.Rows {
		return mockDB.NewRows([]string{"pkey", "kversion", "val"})
	}

	for _, tc := range []struct { //nolint:paralleltest
		name  string
		query string
		args  []sqldriver.Value
		run   func(store *common2.VaultStore) error
	}{
		{
			name:  "GetStates two keys",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2,$3))",
			args:  []sqldriver.Value{"ns", "k1", "k2"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetStates(context.Background(), "ns", "k1", "k2")
				return err
			},
		},
		{
			name:  "GetStates one key",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2))",
			args:  []sqldriver.Value{"ns", "k1"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetStates(context.Background(), "ns", "k1")
				return err
			},
		},
		{
			name:  "GetStateRange both bounds",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3))",
			args:  []sqldriver.Value{"ns", "a", "z"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetStateRange(context.Background(), "ns", "a", "z")
				return err
			},
		},
		{
			name:  "GetStateRange no bounds",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND (1=1))",
			args:  []sqldriver.Value{"ns"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetStateRange(context.Background(), "ns", "", "")
				return err
			},
		},
		{
			name:  "GetAllStates",
			query: "SELECT pkey, kversion, val FROM state WHERE ns = $1",
			args:  []sqldriver.Value{"ns"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllStates(context.Background(), "ns")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			RegisterTestingT(t)
			store, mockDB := newVaultMock(t)
			mockDB.ExpectQuery(tc.query).WithArgs(tc.args...).WillReturnRows(stateRows(mockDB))
			Expect(tc.run(store)).To(Succeed())
			Expect(mockDB.ExpectationsWereMet()).To(Succeed())
		})
	}
}

func TestVaultSQL_GetStateMetadata(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)
	store, mockDB := newVaultMock(t)

	mockDB.
		ExpectQuery("SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)").
		WithArgs("ns", "k1").
		WillReturnRows(mockDB.NewRows([]string{"metadata", "kversion"}))

	_, _, err := store.GetStateMetadata(context.Background(), "ns", "k1")
	Expect(err).ToNot(HaveOccurred())
	Expect(mockDB.ExpectationsWereMet()).To(Succeed())
}

func TestVaultSQL_QueryStatus(t *testing.T) { //nolint:paralleltest
	RegisterTestingT(t)

	keysetFirst := func() driver.Pagination {
		k, err := pagination.KeysetWithField[string](0, 5, "tx_id", "TxID")
		Expect(err).ToNot(HaveOccurred())
		return k
	}
	keysetWithCursor := func() driver.Pagination {
		k, err := pagination.KeysetWithField[string](7, 5, "tx_id", "TxID")
		Expect(err).ToNot(HaveOccurred())
		k.FirstID = "tx3"
		return k
	}
	keysetWithOffset := func() driver.Pagination {
		k, err := pagination.KeysetWithField[int](3, 5, "pos", "Pos")
		Expect(err).ToNot(HaveOccurred())
		return k
	}
	offsetPag := func(os, size int) driver.Pagination {
		p, err := pagination.Offset(os, size)
		Expect(err).ToNot(HaveOccurred())
		return p
	}

	for _, tc := range []struct { //nolint:paralleltest
		name  string
		query string
		args  []sqldriver.Value
		run   func(store *common2.VaultStore) error
	}{
		{
			name:  "GetLast",
			query: "SELECT tx_id, code, message FROM status WHERE pos=(SELECT max(pos) FROM status WHERE code!=$1)",
			args:  []sqldriver.Value{int64(driver.Busy)},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetLast(context.Background())
				return err
			},
		},
		{
			name:  "GetTxStatus",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id = $1",
			args:  []sqldriver.Value{"tx1"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetTxStatus(context.Background(), "tx1")
				return err
			},
		},
		{
			name:  "GetTxStatuses",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id IN ($1,$2)",
			args:  []sqldriver.Value{"tx1", "tx2"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetTxStatuses(context.Background(), "tx1", "tx2")
				return err
			},
		},
		{
			name:  "GetAllTxStatuses none",
			query: "SELECT tx_id, code, message FROM status",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), pagination.None())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses empty",
			query: "SELECT tx_id, code, message FROM status LIMIT 0",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), pagination.Empty())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses offset with skip",
			query: "SELECT tx_id, code, message FROM status LIMIT 5 OFFSET 10",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), offsetPag(10, 5))
				return err
			},
		},
		{
			name:  "GetAllTxStatuses offset without skip",
			query: "SELECT tx_id, code, message FROM status LIMIT 5",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), offsetPag(0, 5))
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset first page",
			query: "SELECT tx_id, code, message FROM status ORDER BY tx_id ASC LIMIT 5",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetFirst())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset with cursor",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id > $1 ORDER BY tx_id ASC LIMIT 5",
			args:  []sqldriver.Value{"tx3"},
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetWithCursor())
				return err
			},
		},
		{
			name:  "GetAllTxStatuses keyset with offset",
			query: "SELECT tx_id, code, message FROM status ORDER BY pos ASC LIMIT 5 OFFSET 3",
			run: func(store *common2.VaultStore) error {
				_, err := store.GetAllTxStatuses(context.Background(), keysetWithOffset())
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { //nolint:paralleltest
			RegisterTestingT(t)
			store, mockDB := newVaultMock(t)
			q := mockDB.ExpectQuery(tc.query)
			if len(tc.args) > 0 {
				q = q.WithArgs(tc.args...)
			}
			q.WillReturnRows(mockDB.NewRows([]string{"tx_id", "code", "message"}))
			Expect(tc.run(store)).To(Succeed())
			Expect(mockDB.ExpectationsWereMet()).To(Succeed())
		})
	}
}
