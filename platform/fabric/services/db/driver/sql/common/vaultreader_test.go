/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"context"
	dbsql "database/sql"
	sqldriver "database/sql/driver"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/pagination"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

var errIsoMap = errors.New("unsupported isolation level")

// failingIsoMapper rejects every isolation level, so buildTxLockVaultReader
// fails before it reaches BeginTx.
type failingIsoMapper struct{}

func (failingIsoMapper) Map(driver.IsolationLevel) (dbsql.IsolationLevel, error) {
	return dbsql.LevelDefault, errIsoMap
}

func newVaultMockWithIso(t *testing.T, il sqlcommon.IsolationLevelMapper) (*fsqlcommon.VaultStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mockDB, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	return fsqlcommon.NewVaultStore(db, db, vaultTables, passthroughWrapper{}, idKey, il), mockDB
}

// defaultIsoMapper maps every isolation level to the default one.
type defaultIsoMapper struct{}

func (defaultIsoMapper) Map(driver.IsolationLevel) (dbsql.IsolationLevel, error) {
	return dbsql.LevelDefault, nil
}

// newVaultMockT is the testify equivalent of the package's newVaultMock, which
// asserts through Gomega and so cannot be called from these tests.
func newVaultMockT(t *testing.T) (*fsqlcommon.VaultStore, sqlmock.Sqlmock) {
	t.Helper()
	return newVaultMockWithIso(t, defaultIsoMapper{})
}

// expectTxLock sets up the status-table insert NewTxLockVaultReader always runs.
func expectTxLock(mockDB sqlmock.Sqlmock, txID string) {
	mockDB.
		ExpectExec("INSERT INTO status (tx_id,code) VALUES ($1,$2) ON CONFLICT DO NOTHING").
		WithArgs(txID, int64(driver.Busy)).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// readerCalls exercises one VaultReader method each, discarding the values so
// the tests below only have to care about the error.
var readerCalls = map[string]func(r driver.LockedVaultReader) error{
	"GetState": func(r driver.LockedVaultReader) error {
		_, err := r.GetState(context.Background(), "ns", "k")
		return err
	},
	"GetStates": func(r driver.LockedVaultReader) error {
		_, err := r.GetStates(context.Background(), "ns", "k")
		return err
	},
	"GetStateRange": func(r driver.LockedVaultReader) error {
		_, err := r.GetStateRange(context.Background(), "ns", "a", "z")
		return err
	},
	"GetAllStates": func(r driver.LockedVaultReader) error {
		_, err := r.GetAllStates(context.Background(), "ns")
		return err
	},
	"GetStateMetadata": func(r driver.LockedVaultReader) error {
		_, _, err := r.GetStateMetadata(context.Background(), "ns", "k")
		return err
	},
	"GetLast": func(r driver.LockedVaultReader) error {
		_, err := r.GetLast(context.Background())
		return err
	},
	"GetTxStatus": func(r driver.LockedVaultReader) error {
		_, err := r.GetTxStatus(context.Background(), "tx1")
		return err
	},
	"GetTxStatuses": func(r driver.LockedVaultReader) error {
		_, err := r.GetTxStatuses(context.Background(), "tx1")
		return err
	},
	"GetAllTxStatuses": func(r driver.LockedVaultReader) error {
		_, err := r.GetAllTxStatuses(context.Background(), pagination.None())
		return err
	},
}

// Every txVaultReader method opens the underlying reader lazily on first use.
// When that open fails, the failure must surface from the method itself — and
// keep surfacing, since once.Do will not retry the open. Done must still be
// safe to call afterwards.
func TestTxVaultReader_LazyOpenFailurePropagates(t *testing.T) {
	t.Parallel()
	for name, call := range readerCalls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, mockDB := newVaultMockWithIso(t, failingIsoMapper{})
			expectTxLock(mockDB, "tx1")

			reader, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
			require.NoError(t, err)

			require.ErrorIs(t, call(reader), errIsoMap)
			require.ErrorIs(t, call(reader), errIsoMap, "the cached open failure must be reported again")
			require.NoError(t, reader.Done())
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

// The reader is opened once and reused: a second call must not begin a second
// transaction.
func TestTxVaultReader_OpensOnce(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	expectTxLock(mockDB, "tx1")
	mockDB.ExpectBegin()
	mockDB.
		ExpectQuery("SELECT tx_id, code, message FROM status WHERE tx_id = $1").
		WithArgs("tx1").
		WillReturnRows(sqlmock.NewRows([]string{"tx_id", "code", "message"}).
			AddRow("tx1", int64(driver.Valid), "ok"))
	mockDB.
		ExpectQuery("SELECT tx_id, code, message FROM status WHERE tx_id = $1").
		WithArgs("tx2").
		WillReturnRows(sqlmock.NewRows([]string{"tx_id", "code", "message"}))
	mockDB.ExpectCommit()

	reader, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
	require.NoError(t, err)

	status, err := reader.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)
	require.Equal(t, "tx1", status.TxID)

	_, err = reader.GetTxStatus(context.Background(), "tx2")
	require.NoError(t, err)

	require.NoError(t, reader.Done())
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// The other side of LazyOpenFailurePropagates: once the reader opens, each
// method must delegate to the underlying vaultReader rather than stop at the
// open. Each case runs against its own reader so the expectations stay
// independent of the order the methods are exercised in.
func TestTxVaultReader_DelegatesAfterOpen(t *testing.T) {
	t.Parallel()
	stateCols := []string{"pkey", "kversion", "val"}
	statusCols := []string{"tx_id", "code", "message"}

	for _, tc := range []struct {
		name  string
		query string
		args  []sqldriver.Value
		cols  []string
		run   func(r driver.LockedVaultReader) error
	}{
		{
			name:  "GetState",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2))",
			args:  []sqldriver.Value{"ns", "k1"},
			cols:  stateCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetState(context.Background(), "ns", "k1")
				return err
			},
		},
		{
			name:  "GetStates",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2,$3))",
			args:  []sqldriver.Value{"ns", "k1", "k2"},
			cols:  stateCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetStates(context.Background(), "ns", "k1", "k2")
				return err
			},
		},
		{
			name:  "GetStateRange",
			query: "SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND (pkey >= $2 AND pkey < $3)) ORDER BY pkey ASC",
			args:  []sqldriver.Value{"ns", "a", "z"},
			cols:  stateCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetStateRange(context.Background(), "ns", "a", "z")
				return err
			},
		},
		{
			name:  "GetAllStates",
			query: "SELECT pkey, kversion, val FROM state WHERE ns = $1",
			args:  []sqldriver.Value{"ns"},
			cols:  stateCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetAllStates(context.Background(), "ns")
				return err
			},
		},
		{
			name:  "GetStateMetadata",
			query: "SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)",
			args:  []sqldriver.Value{"ns", "k1"},
			cols:  []string{"metadata", "kversion"},
			run: func(r driver.LockedVaultReader) error {
				_, _, err := r.GetStateMetadata(context.Background(), "ns", "k1")
				return err
			},
		},
		{
			name:  "GetTxStatus",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id = $1",
			args:  []sqldriver.Value{"tx1"},
			cols:  statusCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetTxStatus(context.Background(), "tx1")
				return err
			},
		},
		{
			name:  "GetTxStatuses",
			query: "SELECT tx_id, code, message FROM status WHERE tx_id IN ($1)",
			args:  []sqldriver.Value{"tx1"},
			cols:  statusCols,
			run: func(r driver.LockedVaultReader) error {
				_, err := r.GetTxStatuses(context.Background(), "tx1")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, mockDB := newVaultMockT(t)
			expectTxLock(mockDB, "tx1")
			mockDB.ExpectBegin()
			mockDB.ExpectQuery(tc.query).WithArgs(tc.args...).
				WillReturnRows(sqlmock.NewRows(tc.cols))
			mockDB.ExpectCommit()

			reader, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
			require.NoError(t, err)

			require.NoError(t, tc.run(reader))
			require.NoError(t, reader.Done())
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestTxVaultReader_BeginFails(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	expectTxLock(mockDB, "tx1")
	beginErr := errors.New("cannot begin")
	mockDB.ExpectBegin().WillReturnError(beginErr)

	reader, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
	require.NoError(t, err)

	_, err = reader.GetLast(context.Background())
	require.ErrorIs(t, err, beginErr)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// Done on an unopened reader is a no-op: newTxVaultReader installs a release
// that does nothing until the real one replaces it.
func TestTxVaultReader_DoneWithoutOpening(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	expectTxLock(mockDB, "tx1")

	reader, err := store.NewTxLockVaultReader(context.Background(), "tx1", driver.LevelDefault)
	require.NoError(t, err)
	require.NoError(t, reader.Done())
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// The global-lock reader takes the store's write lock on open and releases it
// on Done, so a second reader can only open after the first is done.
func TestGlobalLockVaultReader_HoldsAndReleasesLock(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	mockDB.
		ExpectQuery("SELECT tx_id, code, message FROM status WHERE tx_id = $1").
		WithArgs("tx1").
		WillReturnRows(sqlmock.NewRows([]string{"tx_id", "code", "message"}))

	reader, err := store.NewGlobalLockVaultReader(context.Background())
	require.NoError(t, err)

	_, err = reader.GetTxStatus(context.Background(), "tx1")
	require.NoError(t, err)

	require.False(t, store.GlobalLock.TryLock(), "lock must be held while the reader is open")
	require.NoError(t, reader.Done())
	require.True(t, store.GlobalLock.TryLock(), "lock must be free after Done")
	store.GlobalLock.Unlock()

	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestVaultStore_Close(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	mockDB.ExpectClose()

	require.NoError(t, store.Close())
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestCreateSchema(t *testing.T) {
	t.Parallel()

	for name, create := range map[string]func(db *dbsql.DB) error{
		"envelope":  func(db *dbsql.DB) error { return mockEnvelopeStore(db).CreateSchema() },
		"endorsetx": func(db *dbsql.DB) error { return mockEndorseTXStore(db).CreateSchema() },
		"metadata":  func(db *dbsql.DB) error { return mockMetadataStore(db).CreateSchema() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, mockDB, err := sqlmock.New()
			require.NoError(t, err)

			// CreateSchema runs its DDL inside a transaction.
			mockDB.ExpectBegin()
			mockDB.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 0))
			mockDB.ExpectCommit()

			require.NoError(t, create(db))
			require.NoError(t, mockDB.ExpectationsWereMet())
		})
	}
}

func TestGetTableNames(t *testing.T) {
	t.Parallel()
	names, err := fsqlcommon.GetTableNames("prefix")
	require.NoError(t, err)
	require.Equal(t, fsqlcommon.TableNames{
		EndorseTx: "prefix_etx",
		Metadata:  "prefix_meta",
		Envelope:  "prefix_env",
		State:     "prefix_vstate",
		Status:    "prefix_vstatus",
	}, names)
}

// An empty prefix falls back to the package default rather than erroring.
func TestGetTableNames_DefaultPrefix(t *testing.T) {
	t.Parallel()
	names, err := fsqlcommon.GetTableNames("")
	require.NoError(t, err)
	require.Equal(t, "fsc_etx", names.EndorseTx)
}

func TestGetTableNames_InvalidPrefix(t *testing.T) {
	t.Parallel()
	_, err := fsqlcommon.GetTableNames("bad-prefix!")
	require.Error(t, err)
}

// params are appended to each name and validated the same way as the prefix,
// so an illegal one fails on the first Format call.
func TestGetTableNames_InvalidParam(t *testing.T) {
	t.Parallel()
	_, err := fsqlcommon.GetTableNames("prefix", "bad-param!")
	require.Error(t, err)
}

// A write with no Raw bytes is a delete: the version is cleared so the row
// records the absence rather than a stale version.
func TestUpsertStates_Delete(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMockT(t)

	_, args, err := store.UpsertStates(driver.Writes{
		"ns": {"k1": driver.VaultValue{Raw: nil, Version: []byte("1")}},
	}, driver.MetaWrites{})
	require.NoError(t, err)
	require.Equal(t, []any{"ns", "k1", []byte(nil), driver.RawVersion(nil), []byte{}}, args)
}

// A metadata write with no matching value write still produces a row, via the
// second loop over metaWrites.
func TestUpsertStates_MetaOnly(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMockT(t)

	query, args, err := store.UpsertStates(driver.Writes{}, driver.MetaWrites{
		"ns": {"k1": driver.VaultMetadataValue{
			Metadata: map[string][]byte{"m": []byte("v")},
			Version:  []byte("1"),
		}},
	})
	require.NoError(t, err)
	require.Contains(t, query, "INSERT INTO state")
	require.Len(t, args, 5)
	require.Equal(t, "ns", args[0])
	require.Equal(t, "k1", args[1])
	// No value write for this key, so the row carries empty value bytes, the
	// version from the metadata write, and the encoded metadata. The encoding
	// itself is covered by the round-trip test in vault_test.go.
	require.Equal(t, []byte{}, args[2])
	require.Equal(t, driver.RawVersion("1"), args[3])
	require.NotEmpty(t, args[4])
}

// A value and its metadata carrying different versions is suspicious but not
// fatal: the row is still written.
func TestUpsertStates_VersionMismatch(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMockT(t)

	_, args, err := store.UpsertStates(driver.Writes{
		"ns": {"k1": driver.VaultValue{Raw: []byte("v"), Version: []byte("1")}},
	}, driver.MetaWrites{
		"ns": {"k1": driver.VaultMetadataValue{
			Metadata: map[string][]byte{"m": []byte("v")},
			Version:  []byte("2"),
		}},
	})
	require.NoError(t, err)
	require.Len(t, args, 5)
}

func TestUpsertStates_NoValues(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMockT(t)

	_, _, err := store.UpsertStates(driver.Writes{}, driver.MetaWrites{})
	require.Error(t, err)
}

// A key with no row is not an error: the vault reports absent metadata as nil.
func TestGetStateMetadata_NoRows(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	mockDB.
		ExpectQuery("SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)").
		WithArgs("ns", "k1").
		WillReturnRows(sqlmock.NewRows([]string{"metadata", "kversion"}))

	meta, version, err := store.GetStateMetadata(context.Background(), "ns", "k1")
	require.NoError(t, err)
	require.Nil(t, meta)
	require.Nil(t, version)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestGetStateMetadata_QueryFails(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	queryErr := errors.New("query failed")
	mockDB.
		ExpectQuery("SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)").
		WithArgs("ns", "k1").
		WillReturnError(queryErr)

	_, _, err := store.GetStateMetadata(context.Background(), "ns", "k1")
	require.ErrorIs(t, err, queryErr)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// Metadata that isn't valid gob surfaces as a decode error rather than a panic.
func TestGetStateMetadata_BadMetadata(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	mockDB.
		ExpectQuery("SELECT metadata, kversion FROM state WHERE (ns = $1 AND pkey = $2)").
		WithArgs("ns", "k1").
		WillReturnRows(sqlmock.NewRows([]string{"metadata", "kversion"}).
			AddRow([]byte{0xff, 0x00, 0x42}, []byte("1")))

	_, _, err := store.GetStateMetadata(context.Background(), "ns", "k1")
	require.Error(t, err)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestQueryState_QueryFails(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	queryErr := errors.New("query failed")
	mockDB.
		ExpectQuery("SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2))").
		WithArgs("ns", "k1").
		WillReturnError(queryErr)

	_, err := store.GetStates(context.Background(), "ns", "k1")
	require.ErrorIs(t, err, queryErr)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// No transaction IDs means no query: the reader short-circuits to an empty
// iterator rather than building a WHERE with an empty IN list.
func TestGetTxStatuses_NoIDs(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)

	it, err := store.GetTxStatuses(context.Background())
	require.NoError(t, err)
	require.NotNil(t, it)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

func TestGetAllTxStatuses_NilPagination(t *testing.T) {
	t.Parallel()
	store, _ := newVaultMockT(t)

	_, err := store.GetAllTxStatuses(context.Background(), nil)
	require.Error(t, err)
}

// GetState surfaces a failure from the underlying GetStates query rather than
// returning a zero read.
func TestGetState_QueryFails(t *testing.T) {
	t.Parallel()
	store, mockDB := newVaultMockT(t)
	queryErr := errors.New("query failed")
	mockDB.
		ExpectQuery("SELECT pkey, kversion, val FROM state WHERE (ns = $1 AND pkey IN ($2))").
		WithArgs("ns", "k1").
		WillReturnError(queryErr)

	_, err := store.GetState(context.Background(), "ns", "k1")
	require.ErrorIs(t, err, queryErr)
	require.NoError(t, mockDB.ExpectationsWereMet())
}

// A connection lost while reading rows reaches the caller through the store's
// SQLErrorWrapper, so the committer sees driver.ErrConnectionLost and retries
// the block instead of degrading it.
func TestVaultReads_RowErrorIsWrapped(t *testing.T) {
	t.Parallel()

	reads := map[string]func(s *fsqlcommon.VaultStore) error{
		"GetTxStatus": func(s *fsqlcommon.VaultStore) error {
			_, err := s.GetTxStatus(context.Background(), "tx")
			return err
		},
		"GetLast": func(s *fsqlcommon.VaultStore) error {
			_, err := s.GetLast(context.Background())
			return err
		},
		"GetTxStatuses": func(s *fsqlcommon.VaultStore) error {
			it, err := s.GetTxStatuses(context.Background(), "tx")
			if err != nil {
				return err
			}
			_, err = iterators.ReadAllPointers(it)
			return err
		},
		"GetStates": func(s *fsqlcommon.VaultStore) error {
			it, err := s.GetStates(context.Background(), "ns", "k")
			if err != nil {
				return err
			}
			_, err = iterators.ReadAllPointers(it)
			return err
		},
		"GetStateRange": func(s *fsqlcommon.VaultStore) error {
			it, err := s.GetStateRange(context.Background(), "ns", "a", "z")
			if err != nil {
				return err
			}
			_, err = iterators.ReadAllPointers(it)
			return err
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db, mockDB, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			store := fsqlcommon.NewVaultStore(db, db, vaultTables, &postgres.ErrorMapper{}, idKey, defaultIsoMapper{})
			mockDB.ExpectQuery("SELECT").WillReturnRows(
				sqlmock.NewRows([]string{"a", "b", "c"}).AddRow("x", "y", "z").RowError(0, io.ErrUnexpectedEOF))

			err = read(store)
			require.ErrorContains(t, err, io.ErrUnexpectedEOF.Error())
			require.ErrorIs(t, err, dbdriver.ErrConnectionLost)
		})
	}
}
