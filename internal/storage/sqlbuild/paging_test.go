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

func TestZeroPagingWritesNothing(t *testing.T) {
	t.Parallel()

	p := sqlbuild.Paging{}
	require.Nil(t, p.Limit)
	require.Zero(t, p.Offset)
	require.Empty(t, p.OrderBy)
	require.Nil(t, p.Where)

	sql, params := sqlbuild.New().WriteString("SELECT * FROM t").WritePaging(p).Build()
	require.Equal(t, "SELECT * FROM t", sql)
	require.Nil(t, params)
}

// The zero value must be the unpaginated read, not "return no rows": a caller
// who sets OrderBy and forgets the limit gets a full result set rather than a
// silently empty one.
func TestPagingZeroValueContributesNothing(t *testing.T) {
	t.Parallel()

	sql, params := sqlbuild.New().
		WriteString("SELECT * FROM t").
		WritePaging(sqlbuild.Paging{OrderBy: "pos"}).
		Build()

	require.Equal(t, "SELECT * FROM t ORDER BY pos ASC", sql)
	require.Nil(t, params)
}

func TestWritePaging(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		pag  sqlbuild.Paging
		sql  string
	}{
		{
			name: "limit zero is still written",
			pag:  sqlbuild.Paging{OrderBy: "", Limit: new(0), Offset: 0},
			sql:  "SELECT * FROM t LIMIT 0",
		},
		{
			name: "limit only",
			pag:  sqlbuild.Paging{Limit: new(5), Offset: 0},
			sql:  "SELECT * FROM t LIMIT 5",
		},
		{
			name: "limit and offset",
			pag:  sqlbuild.Paging{Limit: new(5), Offset: 10},
			sql:  "SELECT * FROM t LIMIT 5 OFFSET 10",
		},
		{
			name: "order by then limit",
			pag:  sqlbuild.Paging{OrderBy: "tx_id", Limit: new(5)},
			sql:  "SELECT * FROM t ORDER BY tx_id ASC LIMIT 5",
		},
		{
			name: "order by, limit and offset",
			pag:  sqlbuild.Paging{OrderBy: "pos", Limit: new(5), Offset: 3},
			sql:  "SELECT * FROM t ORDER BY pos ASC LIMIT 5 OFFSET 3",
		},
		{
			name: "order by without limit",
			pag:  sqlbuild.Paging{OrderBy: "pos"},
			sql:  "SELECT * FROM t ORDER BY pos ASC",
		},
		{
			name: "offset is ignored without a limit",
			pag:  sqlbuild.Paging{Offset: 10},
			sql:  "SELECT * FROM t",
		},
		{
			// No negative row count is meaningful, and none may reach the
			// database as a literal LIMIT.
			name: "a negative limit writes no LIMIT clause",
			pag:  sqlbuild.Paging{OrderBy: "pos", Limit: new(-5), Offset: 7},
			sql:  "SELECT * FROM t ORDER BY pos ASC",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sql, params := sqlbuild.New().WriteString("SELECT * FROM t").WritePaging(tc.pag).Build()
			require.Equal(t, tc.sql, sql)
			// LIMIT and OFFSET are literals, never parameters: squirrel emitted
			// them as literals and the pinned SQL depends on that.
			require.Nil(t, params)
		})
	}
}

func TestPagingWhereIsTheCallersJob(t *testing.T) {
	t.Parallel()

	// The cursor condition goes through WriteWhere so it lands before ORDER BY
	// and its parameters are numbered after the base condition's.
	pag := sqlbuild.Paging{OrderBy: "tx_id", Limit: new(5), Where: sqlbuild.Gt("tx_id", "tx3")}
	sql, params := sqlbuild.New().
		WriteString("SELECT * FROM t").
		WriteWhere(sqlbuild.Eq("code", 1), pag.Where).
		WritePaging(pag).
		Build()

	require.Equal(t, "SELECT * FROM t WHERE code = $1 AND tx_id > $2 ORDER BY tx_id ASC LIMIT 5", sql)
	require.Equal(t, []sqlbuild.Param{1, "tx3"}, params)
}
