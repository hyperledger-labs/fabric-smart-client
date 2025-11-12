/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"
	"time"

	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"

	"github.com/stretchr/testify/require"
)

func newBindingStoreForTests(t *testing.T) (func(), *BindingStore) {
	// When running this test together with other tests; it may happen that a container instance is still running
	// we give this test a slow start ...
	time.Sleep(10 * time.Second)
	t.Log("starting postgres")
	terminate, pgConnStr, err := StartPostgres(t, false)
	require.NoError(t, err)
	t.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	db, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewBindingStore)
	require.NoError(t, err)
	return terminate, db
}

func TestPutBindingsMultipleEphemeralsPostgres(t *testing.T) {
	// if this test runs after another postgres test, wait a bit to let the previous connections close
	time.Sleep(10 * time.Second)
	terminate, db := newBindingStoreForTests(t)
	defer terminate()
	common.TestPutBindingsMultipleEphemeralsCommon(t, db.BindingStore)
}

func TestManyManyPutBindingsPostgres(t *testing.T) {
	// if this test runs after another postgres test, wait a bit to let the previous connections close
	time.Sleep(10 * time.Second)
	terminate, db := newBindingStoreForTests(t)
	defer terminate()
	common.TestManyManyPutBindingsCommon(t, db.BindingStore)
}
