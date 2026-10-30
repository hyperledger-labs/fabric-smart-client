/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// tempDataSource returns a data source for a fresh database file under t's temp dir.
func tempDataSource(t *testing.T, name string) string {
	t.Helper()

	return fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(t.TempDir(), name))
}

// driverConfig returns a driver.Config whose persistence options unmarshal to c.
func driverConfig(c Config) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, v any) error {
		ptr, ok := v.(*Config)
		if !ok {
			return errors.Errorf("unexpected target type [%T]", v)
		}
		*ptr = c
		return nil
	})

	return cp
}

// TestDriverKVS runs the shared key-value suite against stores built through NewDriver, so
// the config path, the DB provider and NewPersistenceWithOpts are exercised end to end.
func TestDriverKVS(t *testing.T) {
	t.Parallel()

	ds := tempDataSource(t, "kvs")
	o := Opts{DataSource: ds}

	common.TestCases(t, func(string) (driver.KeyValueStore, error) {
		// A fresh driver per store, as the Postgres test does, so each case gets its own
		// connection: the suite closes every store it is given.
		return NewDriver(driverConfig(Config{DataSource: ds})).NewKVS("")
	}, func(string) (driver.UnversionedNotifier, error) {
		dbs, err := open(o)
		require.NoError(t, err)
		p, err := NewKeyValueStoreNotifier(dbs, "test")
		require.NoError(t, err)
		require.NoError(t, p.Persistence.(*KeyValueStore).CreateSchema())

		return p, nil
	}, func(p driver.KeyValueStore) *common.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
	})
}

// TestDriverBinding runs the shared binding suites against a store built through NewDriver.
func TestDriverBinding(t *testing.T) {
	t.Parallel()

	d := NewDriver(driverConfig(Config{DataSource: tempDataSource(t, "binding")}))

	s, err := d.NewBinding("")
	require.NoError(t, err)

	store, ok := s.(*BindingStore)
	require.True(t, ok, "driver returns the sqlite binding store")

	common.TestPutBindingsMultipleEphemeralsCommon(t, store.BindingStore)
	common.TestManyManyPutBindingsCommon(t, store.BindingStore)
}

// TestNewNamedDriver checks the driver registers under the sqlite persistence type.
func TestNewNamedDriver(t *testing.T) {
	t.Parallel()

	nd := NewNamedDriver(driverConfig(Config{DataSource: tempDataSource(t, "named")}), NewDbProvider())

	assert.Equal(t, Persistence, nd.Name)
	assert.NotNil(t, nd.Driver)
}

// TestDriverReportsMissingDataSource checks a config without a data source is reported by
// every store constructor rather than opening a database.
func TestDriverReportsMissingDataSource(t *testing.T) {
	t.Parallel()

	d := NewDriver(driverConfig(Config{}))

	_, err := d.NewKVS("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing data source")
}

// schemaStore records how NewPersistenceWithOpts prepared its schema.
type schemaStore struct {
	table        string
	createSchema int
}

func (s *schemaStore) CreateSchema() error { s.createSchema++; return nil }

// versionedSchemaStore is a schemaStore whose schema migrate.Migrate manages.
type versionedSchemaStore struct{ *schemaStore }

func (s versionedSchemaStore) StoreName() string { return s.table }
func (s versionedSchemaStore) Steps() []migrate.Step {
	return []migrate.Step{{Version: 1, Name: "create", SQL: []string{"CREATE TABLE " + s.table + " (id INT)"}}}
}

// newGenericStore is a constructor generic over DBObject, as applications such
// as Panurus write them; it must keep compiling against NewPersistenceWithOpts.
func newGenericStore[V drivercommon.DBObject](wrap func(*schemaStore) V) common.PersistenceConstructor[V] {
	return func(_ *drivercommon.RWDB, tables common.TableNames) (V, error) {
		return wrap(&schemaStore{table: tables.KVS}), nil
	}
}

// TestNewPersistenceWithOptsSchema checks how NewPersistenceWithOpts prepares a
// store's schema: a plain DBObject gets CreateSchema, a migrate.Versioned store
// gets its steps and a row in the versions table, and SkipCreateTable does
// neither.
func TestNewPersistenceWithOptsSchema(t *testing.T) {
	t.Parallel()

	open := func(t *testing.T, c Config, wrap func(*schemaStore) drivercommon.DBObject) drivercommon.DBObject {
		t.Helper()
		p, err := NewPersistenceWithOpts(NewConfigProvider(drivertesting.MockConfig(c)), NewDbProvider(), "", newGenericStore(wrap))
		require.NoError(t, err)
		return p
	}
	plain := func(s *schemaStore) drivercommon.DBObject { return s }
	versioned := func(s *schemaStore) drivercommon.DBObject { return versionedSchemaStore{s} }

	t.Run("plain store gets CreateSchema", func(t *testing.T) {
		t.Parallel()
		p := open(t, Config{DataSource: tempDataSource(t, "plain")}, plain)
		assert.Equal(t, 1, p.(*schemaStore).createSchema)
	})

	t.Run("SkipCreateTable skips CreateSchema", func(t *testing.T) {
		t.Parallel()
		p := open(t, Config{DataSource: tempDataSource(t, "skip"), SkipCreateTable: true}, plain)
		assert.Zero(t, p.(*schemaStore).createSchema)
	})

	t.Run("versioned store is migrated", func(t *testing.T) {
		t.Parallel()
		ds := tempDataSource(t, "versioned")
		p := open(t, Config{DataSource: ds, TablePrefix: "App"}, versioned)
		assert.Zero(t, p.(versionedSchemaStore).createSchema)

		dbs, err := NewDbProvider().Get(Opts{DataSource: ds})
		require.NoError(t, err)
		var version int
		require.NoError(t, dbs.ReadDB.QueryRow("SELECT version FROM app_schema_versions WHERE store = 'app_kvs'").Scan(&version))
		assert.Equal(t, 1, version)
	})

	t.Run("SkipCreateTable skips Migrate", func(t *testing.T) {
		t.Parallel()
		ds := tempDataSource(t, "versionedskip")
		open(t, Config{DataSource: ds, SkipCreateTable: true}, versioned)

		dbs, err := NewDbProvider().Get(Opts{DataSource: ds})
		require.NoError(t, err)
		var n int
		require.NoError(t, dbs.ReadDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name LIKE '%schema_versions'").Scan(&n))
		assert.Zero(t, n)
	})
}
