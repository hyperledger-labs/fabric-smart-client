/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	fabricdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	viewdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	commondriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

func TestGetOpts_Fields(t *testing.T) {
	t.Parallel()
	opts := mem.Op.GetOpts()
	require.Equal(t, "file::memory:?cache=shared", opts.DataSource)
	require.False(t, opts.SkipPragmas)
	require.Equal(t, 10, opts.MaxOpenConns)
	require.Equal(t, commondriver.DefaultMaxIdleConns, opts.MaxIdleConns)
	require.Equal(t, commondriver.DefaultMaxIdleTime, opts.MaxIdleTime)
	require.Empty(t, opts.TablePrefix)
	require.Empty(t, opts.TableNameParams)
	require.Nil(t, opts.Tracing)
}

func TestGetOpts_WithParams(t *testing.T) {
	t.Parallel()
	opts := mem.Op.GetOpts("p1", "p2")
	require.Equal(t, []string{"p1", "p2"}, opts.TableNameParams)
}

func TestGetConfig_Fields(t *testing.T) {
	t.Parallel()
	cfg := mem.Op.GetConfig()
	require.Equal(t, "file::memory:?cache=shared", cfg.DataSource)
	require.False(t, cfg.SkipPragmas)
	require.Equal(t, 10, cfg.MaxOpenConns)
	require.NotNil(t, cfg.MaxIdleConns)
	require.Equal(t, commondriver.DefaultMaxIdleConns, *cfg.MaxIdleConns)
	require.NotNil(t, cfg.MaxIdleTime)
	require.Equal(t, commondriver.DefaultMaxIdleTime, *cfg.MaxIdleTime)
	require.False(t, cfg.SkipCreateTable)
	require.Empty(t, cfg.TablePrefix)
	require.Empty(t, cfg.TableNameParams)
}

func TestGetConfig_WithParams(t *testing.T) {
	t.Parallel()
	cfg := mem.Op.GetConfig("a", "b")
	require.Equal(t, []string{"a", "b"}, cfg.TableNameParams)
}

func TestNewDriver_NotNil(t *testing.T) {
	t.Parallel()
	d := mem.NewDriver()
	require.NotNil(t, d)
}

func TestNewDriverWithDbProvider_NotNil(t *testing.T) {
	t.Parallel()
	d := mem.NewDriverWithDbProvider(sqlite2.NewDbProvider())
	require.NotNil(t, d)
}

func TestNewEndorseTx_CRUD(t *testing.T) { //nolint:paralleltest
	d := mem.NewDriver()
	store, err := d.NewEndorseTx(viewdriver.PersistenceName(t.Name()), t.Name())
	require.NoError(t, err)
	require.NotNil(t, store)

	ctx := t.Context()
	key := "tx1"

	exists, err := store.ExistsEndorseTx(ctx, key)
	require.NoError(t, err)
	require.False(t, exists)

	err = store.PutEndorseTx(ctx, key, []byte("payload"))
	require.NoError(t, err)

	exists, err = store.ExistsEndorseTx(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)

	val, err := store.GetEndorseTx(ctx, key)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), val)
}

func TestNewMetadata_CRUD(t *testing.T) { //nolint:paralleltest
	d := mem.NewDriver()
	store, err := d.NewMetadata(viewdriver.PersistenceName(t.Name()), t.Name())
	require.NoError(t, err)
	require.NotNil(t, store)

	ctx := t.Context()
	key := "meta1"

	exists, err := store.ExistMetadata(ctx, key)
	require.NoError(t, err)
	require.False(t, exists)

	err = store.PutMetadata(ctx, key, []byte("meta-value"))
	require.NoError(t, err)

	exists, err = store.ExistMetadata(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)

	val, err := store.GetMetadata(ctx, key)
	require.NoError(t, err)
	require.Equal(t, []byte("meta-value"), val)
}

func TestNewEnvelope_CRUD(t *testing.T) { //nolint:paralleltest
	d := mem.NewDriver()
	store, err := d.NewEnvelope(viewdriver.PersistenceName(t.Name()), t.Name())
	require.NoError(t, err)
	require.NotNil(t, store)

	ctx := t.Context()
	key := "env1"

	exists, err := store.ExistsEnvelope(ctx, key)
	require.NoError(t, err)
	require.False(t, exists)

	err = store.PutEnvelope(ctx, key, []byte("env-data"))
	require.NoError(t, err)

	exists, err = store.ExistsEnvelope(ctx, key)
	require.NoError(t, err)
	require.True(t, exists)

	val, err := store.GetEnvelope(ctx, key)
	require.NoError(t, err)
	require.Equal(t, []byte("env-data"), val)
}

func TestNewVault_CRUD(t *testing.T) { //nolint:paralleltest
	d := mem.NewDriver()
	store, err := d.NewVault(viewdriver.PersistenceName(t.Name()), t.Name())
	require.NoError(t, err)
	require.NotNil(t, store)

	ctx := t.Context()
	txID := "tx1"

	// Confirm clean state before writing
	status, err := store.GetTxStatus(ctx, txID)
	require.NoError(t, err)
	require.Nil(t, status)

	// Write status
	err = store.SetStatuses(ctx, fabricdriver.Valid, "ok", txID)
	require.NoError(t, err)

	// Read status
	status, err = store.GetTxStatus(ctx, txID)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.Equal(t, fabricdriver.Valid, status.Code)
}

func TestNewEndorseTx_MultipleInstances(t *testing.T) { //nolint:paralleltest
	d := mem.NewDriver()

	store1, err := d.NewEndorseTx(viewdriver.PersistenceName(t.Name()+"_1"), "alpha")
	require.NoError(t, err)

	store2, err := d.NewEndorseTx(viewdriver.PersistenceName(t.Name()+"_2"), "beta")
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, store1.PutEndorseTx(ctx, "k", []byte("v1")))
	require.NoError(t, store2.PutEndorseTx(ctx, "k", []byte("v2")))

	val1, err := store1.GetEndorseTx(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v1"), val1)

	val2, err := store2.GetEndorseTx(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, []byte("v2"), val2)
}
