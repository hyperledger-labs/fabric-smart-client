/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// Fakes for error injection testing

type fakeDbProvider struct {
	err error
}

func (f *fakeDbProvider) Get(sqlite.Opts) (*common.RWDB, error) {
	if f.err != nil {
		return nil, f.err
	}
	return sqlite.NewDbProvider().Get(Op.GetOpts())
}

type fakePersistence struct {
	createSchemaErr error
}

func (f *fakePersistence) CreateSchema() error {
	return f.createSchemaErr
}

func (f *fakePersistence) Close() error {
	return nil
}

func (f *fakePersistence) BeginUpdate() error {
	return nil
}

func (f *fakePersistence) Commit() error {
	return nil
}

func (f *fakePersistence) Discard() error {
	return nil
}

func (f *fakePersistence) Stats() any {
	return nil
}

var _ common.DBObject = (*fakePersistence)(nil)

func TestNewNamedDriver(t *testing.T) {
	t.Parallel()

	dbProvider := sqlite.NewDbProvider()
	namedDriver := NewNamedDriver(dbProvider)

	require.Equal(t, Persistence, namedDriver.Name)
	require.NotNil(t, namedDriver.Driver)
	require.IsType(t, &Driver{}, namedDriver.Driver)
}

func TestNewDriver(t *testing.T) {
	t.Parallel()

	d := NewDriver()

	require.NotNil(t, d)
	require.NotNil(t, d.dbProvider)
}

func TestNewDriverWithDbProvider(t *testing.T) {
	t.Parallel()

	dbProvider := sqlite.NewDbProvider()
	d := NewDriverWithDbProvider(dbProvider)

	require.NotNil(t, d)
	require.Equal(t, dbProvider, d.dbProvider)
}

//nolint:paralleltest,tparallel
func TestDriver_NewKVS(t *testing.T) {
	t.Run("success without params", func(t *testing.T) {
		d := NewDriver()
		kvs, err := d.NewKVS(driver.PersistenceName(t.Name()))

		require.NoError(t, err)
		require.NotNil(t, kvs)
		require.NoError(t, kvs.Close())
	})

	t.Run("success with params", func(t *testing.T) {
		d := NewDriver()
		kvs, err := d.NewKVS(driver.PersistenceName(t.Name()), "alice", "bob")

		require.NoError(t, err)
		require.NotNil(t, kvs)
		require.NoError(t, kvs.Close())
	})

	t.Run("error from dbProvider", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("db provider error")
		fakeProvider := &fakeDbProvider{err: expectedErr}
		d := NewDriverWithDbProvider(fakeProvider)

		kvs, err := d.NewKVS("test")

		require.Error(t, err)
		require.Nil(t, kvs)
		require.Contains(t, err.Error(), "error opening db")
	})
}

//nolint:paralleltest,tparallel
func TestDriver_NewBinding(t *testing.T) {
	cases := []struct {
		name   string
		params []string
	}{
		{name: "success without params"},
		{name: "success with params", params: []string{"alice"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriverWithDbProvider(sqlite.NewDbProvider())
			bs, err := d.NewBinding(driver.PersistenceName(t.Name()), tc.params...)

			require.NoError(t, err)
			require.NotNil(t, bs)
		})
	}

	t.Run("error from dbProvider", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("db provider error")
		fakeProvider := &fakeDbProvider{err: expectedErr}
		d := NewDriverWithDbProvider(fakeProvider)

		bs, err := d.NewBinding("test")

		require.Error(t, err)
		require.Nil(t, bs)
		require.Contains(t, err.Error(), "error opening db")
	})
}

//nolint:paralleltest,tparallel
func TestDriver_NewSignerInfo(t *testing.T) {
	cases := []struct {
		name   string
		params []string
	}{
		{name: "success without params"},
		{name: "success with params", params: []string{"alice", "bob"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriverWithDbProvider(sqlite.NewDbProvider())
			sis, err := d.NewSignerInfo(driver.PersistenceName(t.Name()), tc.params...)

			require.NoError(t, err)
			require.NotNil(t, sis)
		})
	}

	t.Run("error from dbProvider", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("db provider error")
		fakeProvider := &fakeDbProvider{err: expectedErr}
		d := NewDriverWithDbProvider(fakeProvider)

		sis, err := d.NewSignerInfo("test")

		require.Error(t, err)
		require.Nil(t, sis)
		require.Contains(t, err.Error(), "error opening db")
	})
}

//nolint:paralleltest,tparallel
func TestDriver_NewAuditInfo(t *testing.T) {
	cases := []struct {
		name   string
		params []string
	}{
		{name: "success without params"},
		{name: "success with params", params: []string{"alice"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDriverWithDbProvider(sqlite.NewDbProvider())
			ais, err := d.NewAuditInfo(driver.PersistenceName(t.Name()), tc.params...)

			require.NoError(t, err)
			require.NotNil(t, ais)
		})
	}

	t.Run("error from dbProvider", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("db provider error")
		fakeProvider := &fakeDbProvider{err: expectedErr}
		d := NewDriverWithDbProvider(fakeProvider)

		ais, err := d.NewAuditInfo("test")

		require.Error(t, err)
		require.Nil(t, ais)
		require.Contains(t, err.Error(), "error opening db")
	})
}

func TestNewPersistenceWithOpts_ConstructorError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("constructor error")
	fakeProvider := &fakeDbProvider{}
	failingConstructor := func(*common.RWDB, common2.TableNames) (*fakePersistence, error) {
		return nil, expectedErr
	}

	result, err := newPersistenceWithOpts(fakeProvider, failingConstructor)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, expectedErr, err)
}

func TestNewPersistenceWithOpts_CreateSchemaError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("create schema error")
	fakeProvider := &fakeDbProvider{}
	failingConstructor := func(*common.RWDB, common2.TableNames) (*fakePersistence, error) {
		return &fakePersistence{createSchemaErr: expectedErr}, nil
	}

	result, err := newPersistenceWithOpts(fakeProvider, failingConstructor)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, expectedErr, err)
}

func TestPersistenceConstant(t *testing.T) {
	t.Parallel()

	require.Equal(t, "memory", string(Persistence))
}
