/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// errFake stands in for any failure the driver only passes through. It is a
// sentinel so the assertions can match it with errors.Is rather than on text.
var errFake = errors.New("fake failure")

// fakeDbProvider hands out a pre-built RWDB instead of dialling a server, so the
// driver surface is exercised without a database. A nil err yields dbs.
type fakeDbProvider struct {
	dbs  *common2.RWDB
	err  error
	opts []postgres2.Opts
}

func (p *fakeDbProvider) Get(o postgres2.Opts) (*common2.RWDB, error) {
	p.opts = append(p.opts, o)
	if p.err != nil {
		return nil, p.err
	}
	return p.dbs, nil
}

// mockDB returns an RWDB whose read and write handles are the same sqlmock
// connection, matching Open's single-handle layout, plus the mock controller.
// The schema transaction CreateSchema runs, and the vault's pkey type and
// primary-key checks, are expected but their statements are not matched: these tests assert on
// construction, not on DDL.
func mockDB(t *testing.T) (*common2.RWDB, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	m.MatchExpectationsInOrder(false)
	m.ExpectBegin()
	m.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectCommit()
	m.ExpectQuery("SELECT format_type").WillReturnRows(sqlmock.NewRows([]string{"format_type"}).AddRow("bytea"))
	m.ExpectQuery("SELECT quote_ident").WillReturnRows(sqlmock.NewRows([]string{"conname", "cols"}).AddRow("vstate_pkey", "ns,pkey"))
	return &common2.RWDB{ReadDB: db, WriteDB: db}, m
}

// validConfig builds a Config the driver accepts: GetOpts only requires a
// non-empty data source, and no connection is made to it.
func validConfig() driver2.Config {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, val any) error {
		o, ok := val.(*postgres2.Config)
		if !ok {
			return nil
		}
		o.DataSource = "postgres://user:pass@localhost:5432/testdb"
		return nil
	})
	return cp
}

// emptyConfig leaves the data source empty, which GetOpts rejects. It is the
// cheapest way to reach every constructor's config-failure branch.
func emptyConfig() driver2.Config {
	return &mock.ConfigProvider{}
}

func TestNewNamedDriver(t *testing.T) {
	t.Parallel()

	nd := NewNamedDriver(validConfig(), &fakeDbProvider{})
	require.Equal(t, Persistence, nd.Name)
	require.Equal(t, driver.PersistenceType("postgres"), nd.Name)
	require.NotNil(t, nd.Driver)
}

func TestNewDriverWithDbProvider(t *testing.T) {
	t.Parallel()

	p := &fakeDbProvider{}
	d := NewDriverWithDbProvider(validConfig(), p)
	require.NotNil(t, d.cp)
	require.Same(t, p, d.dbProvider)
}

// The four store constructors share NewPersistenceWithOpts, so they are covered
// as a table over the Driver methods rather than one test each.
func TestDriverStoreConstructors(t *testing.T) {
	t.Parallel()

	for name, newStore := range map[string]func(*Driver) (any, error){
		"EndorseTx": func(d *Driver) (any, error) { return d.NewEndorseTx("") },
		"Metadata":  func(d *Driver) (any, error) { return d.NewMetadata("") },
		"Envelope":  func(d *Driver) (any, error) { return d.NewEnvelope("") },
		"Vault":     func(d *Driver) (any, error) { return d.NewVault("") },
	} {
		t.Run(name+"/valid", func(t *testing.T) {
			t.Parallel()
			dbs, _ := mockDB(t)
			s, err := newStore(NewDriverWithDbProvider(validConfig(), &fakeDbProvider{dbs: dbs}))
			require.NoError(t, err)
			require.NotNil(t, s)
		})

		t.Run(name+"/invalid config", func(t *testing.T) {
			t.Parallel()
			dbs, _ := mockDB(t)
			_, err := newStore(NewDriverWithDbProvider(emptyConfig(), &fakeDbProvider{dbs: dbs}))
			require.ErrorContains(t, err, "missing data source")
		})

		t.Run(name+"/db open failure", func(t *testing.T) {
			t.Parallel()
			p := &fakeDbProvider{err: errFake}
			_, err := newStore(NewDriverWithDbProvider(validConfig(), p))
			require.ErrorContains(t, err, "error opening db")
			require.ErrorIs(t, err, errFake)
		})
	}
}

// The params passed to a Driver method become the table-name parameters, and
// reach the provider as Opts.TableNameParams.
func TestNewPersistenceWithOptsPassesParams(t *testing.T) {
	t.Parallel()

	dbs, _ := mockDB(t)
	p := &fakeDbProvider{dbs: dbs}
	d := NewDriverWithDbProvider(validConfig(), p)

	_, err := d.NewEndorseTx("", "chan", "ns")
	require.NoError(t, err)
	require.Len(t, p.opts, 1)
	require.Equal(t, []string{"chan", "ns"}, p.opts[0].TableNameParams)
	require.Equal(t, "postgres://user:pass@localhost:5432/testdb", p.opts[0].DataSource)
}

// SkipCreateTable short-circuits CreateSchema, so no statement reaches the
// database. The mock is deliberately given no expectations: any Begin or Exec
// then fails the read, and ExpectationsWereMet confirms nothing ran at all.
func TestNewPersistenceWithOptsSkipCreateTable(t *testing.T) {
	t.Parallel()

	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, val any) error {
		o, ok := val.(*postgres2.Config)
		if !ok {
			return nil
		}
		o.DataSource = "postgres://localhost:5432/testdb"
		o.SkipCreateTable = true
		return nil
	})

	db, m, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	d := NewDriverWithDbProvider(cp, &fakeDbProvider{dbs: &common2.RWDB{ReadDB: db, WriteDB: db}})
	store, err := d.NewVault("")
	require.NoError(t, err)
	require.NotNil(t, store)
	require.NoError(t, m.ExpectationsWereMet())
}

// An unusable TablePrefix fails table-name computation after the database is
// already open, so the error surfaces from GetTableNames rather than from
// GetOpts or the provider.
func TestNewPersistenceWithOptsBadTablePrefix(t *testing.T) {
	t.Parallel()

	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, val any) error {
		o, ok := val.(*postgres2.Config)
		if !ok {
			return nil
		}
		o.DataSource = "postgres://localhost:5432/testdb"
		// Only letters and underscores are legal in a table prefix.
		o.TablePrefix = "bad-prefix"
		return nil
	})

	dbs, _ := mockDB(t)
	d := NewDriverWithDbProvider(cp, &fakeDbProvider{dbs: dbs})

	_, err := d.NewEndorseTx("")
	require.ErrorContains(t, err, "illegal character in table prefix")
}

// A constructor error propagates unwrapped, and stops CreateSchema from running.
func TestNewPersistenceWithOptsConstructorError(t *testing.T) {
	t.Parallel()

	dbs, _ := mockDB(t)
	d := NewDriverWithDbProvider(validConfig(), &fakeDbProvider{dbs: dbs})

	_, err := NewPersistenceWithOpts(d.cp, d.dbProvider, "",
		func(*common2.RWDB, common3.TableNames) (*VaultStore, error) { return nil, errFake })
	require.ErrorIs(t, err, errFake)
}

// The build* helpers pick one table out of TableNames per store. The chosen
// table is held in an unexported field, so each store is asserted through the
// query it issues, which is the only place the choice is observable — and the
// only thing that would catch a store wired to a sibling's table.
func TestBuildStores(t *testing.T) {
	t.Parallel()

	tables, err := common3.GetTableNames("test", "chan")
	require.NoError(t, err)

	// Every store here reads through SimpleKeyDataStore, so one read is enough
	// to reveal which table it was given.
	for name, read := range map[string]struct {
		table string
		get   func(*common2.RWDB) error
	}{
		"EndorseTx": {tables.EndorseTx, func(dbs *common2.RWDB) error {
			s, err := NewEndorseTxStore(dbs, tables)
			require.NoError(t, err)
			_, err = s.GetEndorseTx(context.Background(), "k")
			return err
		}},
		"Envelope": {tables.Envelope, func(dbs *common2.RWDB) error {
			s, err := NewEnvelopeStore(dbs, tables)
			require.NoError(t, err)
			_, err = s.GetEnvelope(context.Background(), "k")
			return err
		}},
		"Metadata": {tables.Metadata, func(dbs *common2.RWDB) error {
			s, err := NewMetadataStore(dbs, tables)
			require.NoError(t, err)
			_, err = s.GetMetadata(context.Background(), "k")
			return err
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			db, m, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })

			m.ExpectQuery("SELECT data FROM " + read.table + " WHERE key = $1").
				WithArgs("k").
				WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow([]byte("v")))

			require.NoError(t, read.get(&common2.RWDB{ReadDB: db, WriteDB: db}))
			// The exact-match expectation above only fails the read; this is
			// what fails the test if the store queried a different table.
			require.NoError(t, m.ExpectationsWereMet())
		})
	}

	// The vault store keeps its two tables in an exported-to-the-package field,
	// so it is checked directly.
	t.Run("Vault", func(t *testing.T) {
		t.Parallel()
		dbs, _ := mockDB(t)
		s, err := NewVaultStore(dbs, tables)
		require.NoError(t, err)
		require.NotNil(t, s.VaultStore)
		require.Equal(t, tables.State, s.tables.StateTable)
		require.Equal(t, tables.Status, s.tables.StatusTable)
		require.Same(t, dbs.WriteDB, s.writeDB)
	})
}

// TestBuildStoresUseReadAndWriteDBs pins which pool each store talks to.
// TestBuildStores passes one mock as both ReadDB and WriteDB, so it cannot see
// the two being swapped; these subtests give the store two distinct mocks and
// fail if a read lands on the write pool or a write on the read pool.
func TestBuildStoresUseReadAndWriteDBs(t *testing.T) {
	t.Parallel()

	tables, err := common3.GetTableNames("test", "chan")
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		table string
		get   func(*common2.RWDB) error
		put   func(*common2.RWDB) error
	}{
		"EndorseTx": {tables.EndorseTx, func(dbs *common2.RWDB) error {
			s, err := NewEndorseTxStore(dbs, tables)
			if err != nil {
				return err
			}
			_, err = s.GetEndorseTx(context.Background(), "k")
			return err
		}, func(dbs *common2.RWDB) error {
			s, err := NewEndorseTxStore(dbs, tables)
			if err != nil {
				return err
			}
			return s.PutEndorseTx(context.Background(), "k", []byte("v"))
		}},
		"Envelope": {tables.Envelope, func(dbs *common2.RWDB) error {
			s, err := NewEnvelopeStore(dbs, tables)
			if err != nil {
				return err
			}
			_, err = s.GetEnvelope(context.Background(), "k")
			return err
		}, func(dbs *common2.RWDB) error {
			s, err := NewEnvelopeStore(dbs, tables)
			if err != nil {
				return err
			}
			return s.PutEnvelope(context.Background(), "k", []byte("v"))
		}},
		"Metadata": {tables.Metadata, func(dbs *common2.RWDB) error {
			s, err := NewMetadataStore(dbs, tables)
			if err != nil {
				return err
			}
			_, err = s.GetMetadata(context.Background(), "k")
			return err
		}, func(dbs *common2.RWDB) error {
			s, err := NewMetadataStore(dbs, tables)
			if err != nil {
				return err
			}
			return s.PutMetadata(context.Background(), "k", []byte("v"))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			readDB, readMock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = readDB.Close() })

			writeDB, writeMock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = writeDB.Close() })

			dbs := &common2.RWDB{ReadDB: readDB, WriteDB: writeDB}

			// A read must go to the read pool. An unexpected call on either mock
			// fails the call itself, so a read sent to writeMock errors here.
			readMock.ExpectQuery("SELECT data FROM " + tc.table).
				WithArgs("k").
				WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow([]byte("v")))
			require.NoError(t, tc.get(dbs))
			require.NoError(t, readMock.ExpectationsWereMet())
			require.NoError(t, writeMock.ExpectationsWereMet())

			// And a write must go to the write pool.
			writeMock.ExpectExec("INSERT INTO " + tc.table).
				WillReturnResult(sqlmock.NewResult(1, 1))
			require.NoError(t, tc.put(dbs))
			require.NoError(t, writeMock.ExpectationsWereMet())
			require.NoError(t, readMock.ExpectationsWereMet())
		})
	}
}
