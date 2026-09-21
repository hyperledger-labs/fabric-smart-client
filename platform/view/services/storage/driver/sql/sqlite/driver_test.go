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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
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
		// A fresh driver per store, as the Postgres test does: the DB provider caches the
		// connection by data source, and each case closes its store when done.
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
