/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

// TestIsolationLevelsMap checks every level pgx supports maps to its sql equivalent,
// and undefined levels and levels pgx does not support are rejected.
func TestIsolationLevelsMap(t *testing.T) {
	t.Parallel()

	supported := map[cdriver.IsolationLevel]sql.IsolationLevel{
		cdriver.LevelDefault:         sql.LevelDefault,
		cdriver.LevelReadUncommitted: sql.LevelReadUncommitted,
		cdriver.LevelReadCommitted:   sql.LevelReadCommitted,
		cdriver.LevelRepeatableRead:  sql.LevelRepeatableRead,
		cdriver.LevelSnapshot:        sql.LevelSnapshot,
		cdriver.LevelSerializable:    sql.LevelSerializable,
	}
	require.Len(t, IsolationLevels, len(supported))
	for level, want := range supported {
		il, err := IsolationLevels.Map(level)
		require.NoError(t, err)
		assert.Equal(t, want, il)
	}

	for _, level := range []cdriver.IsolationLevel{cdriver.LevelWriteCommitted, cdriver.LevelLinearizable, 99} {
		_, err := IsolationLevels.Map(level)
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("[%d]", level), "the requested level is reported")
	}
}

// TestPostgresIsolationLevelsBeginTx checks every level the mapper accepts also
// begins a read-only transaction through pgx, as the vault store does.
func TestPostgresIsolationLevelsBeginTx(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("pgx", setupDB(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for level, il := range IsolationLevels {
		tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: il, ReadOnly: true})
		require.NoError(t, err, "level [%d] maps to %s", level, il)
		require.NoError(t, tx.Commit())
	}
}
