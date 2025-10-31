/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/require"
)

func TestPutBindingsMultipleEphemerals(t *testing.T) {
	t.Log("starting postgres")
	terminate, pgConnStr, err := StartPostgres(t, false)
	require.NoError(t, err)
	defer terminate()
	t.Log("postgres ready")

	cp := NewConfigProvider(testing2.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	db, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewBindingStore)
	require.NoError(t, err)

	ctx := context.Background()

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")

	// Check that store does not have bindings for e1 and e2
	lt, err := db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)
	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, len(lt), 0)

	// Create new bindings
	err = db.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)

	// Check that the bindings where correctly written
	lt, err = db.GetLongTerm(ctx, e1)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)

	lt, err = db.GetLongTerm(ctx, e2)
	require.NoError(t, err)
	require.ElementsMatch(t, lt, longTerm)
}
