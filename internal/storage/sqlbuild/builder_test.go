/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlbuild_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
)

func TestBuilderNumbersParamsInWriteOrder(t *testing.T) {
	t.Parallel()

	sql, params := sqlbuild.New().
		WriteString("SELECT val FROM t WHERE ns = ").
		WriteParam("ns").
		WriteString(" AND pkey = ").
		WriteParam([]byte("k")).
		Build()

	require.Equal(t, "SELECT val FROM t WHERE ns = $1 AND pkey = $2", sql)
	require.Equal(t, []sqlbuild.Param{"ns", []byte("k")}, params)
}

func TestBuilderEmptyHasNoParams(t *testing.T) {
	t.Parallel()

	sql, params := sqlbuild.New().WriteString("SELECT * FROM t").Build()

	require.Equal(t, "SELECT * FROM t", sql)
	// nil, not an empty slice: callers pass this straight to QueryContext and
	// the pinned squirrel behaviour is a nil arg slice for parameterless queries.
	require.Nil(t, params)
}

func TestBuilderWriteTuples(t *testing.T) {
	t.Parallel()

	sql, params := sqlbuild.New().
		WriteString("INSERT INTO t (a,b) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{"a1", 1}, {"a2", 2}}).
		WriteString(" ON CONFLICT DO NOTHING").
		Build()

	// no space after the comma: this matches squirrel's output exactly
	require.Equal(t, "INSERT INTO t (a,b) VALUES ($1,$2),($3,$4) ON CONFLICT DO NOTHING", sql)
	require.Equal(t, []sqlbuild.Param{"a1", 1, "a2", 2}, params)
}

func TestBuilderWriteTuplesSingleRow(t *testing.T) {
	t.Parallel()

	sql, _ := sqlbuild.New().
		WriteString("INSERT INTO t (a,b,c) VALUES ").
		WriteTuples([]sqlbuild.Tuple{{"a", "b", "c"}}).
		Build()

	require.Equal(t, "INSERT INTO t (a,b,c) VALUES ($1,$2,$3)", sql)
}

// WriteTuples with no rows would render "VALUES " and hand invalid SQL to the
// database. Callers reject an empty row set with ErrNoValues before building, so
// reaching the builder with one is a programming error, not a runtime condition.
func TestWriteTuplesRejectsNoRows(t *testing.T) {
	t.Parallel()

	for _, rows := range [][]sqlbuild.Tuple{nil, {}} {
		require.PanicsWithValue(t,
			"sqlbuild: WriteTuples called with no rows; callers must reject this with ErrNoValues",
			func() {
				sqlbuild.New().
					WriteString("INSERT INTO t (a) VALUES ").
					WriteTuples(rows).
					Build()
			})
	}
}

func TestBuilderParamsContinueAcrossSections(t *testing.T) {
	t.Parallel()

	// mirrors UPDATE ... SET code = $1 WHERE tx_id IN ($2,$3): the SET param is
	// numbered before the WHERE params because it is written first.
	sql, params := sqlbuild.New().
		WriteString("UPDATE t SET code = ").
		WriteParam(1).
		WriteString(" WHERE tx_id IN (").
		WriteParam("tx1").
		WriteString(",").
		WriteParam("tx2").
		WriteString(")").
		Build()

	require.Equal(t, "UPDATE t SET code = $1 WHERE tx_id IN ($2,$3)", sql)
	require.Equal(t, []sqlbuild.Param{1, "tx1", "tx2"}, params)
}
