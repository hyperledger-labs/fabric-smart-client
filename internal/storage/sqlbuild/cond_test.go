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

func render(t *testing.T, c sqlbuild.Condition) (string, []sqlbuild.Param) {
	t.Helper()
	b := sqlbuild.New()
	c.WriteTo(b)
	return b.Build()
}

func TestConditions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		cond   sqlbuild.Condition
		sql    string
		params []sqlbuild.Param
	}{
		{
			name:   "Eq is bare",
			cond:   sqlbuild.Eq("ns", "n"),
			sql:    "ns = $1",
			params: []sqlbuild.Param{"n"},
		},
		{
			name:   "Gt",
			cond:   sqlbuild.Gt("tx_id", "tx3"),
			sql:    "tx_id > $1",
			params: []sqlbuild.Param{"tx3"},
		},
		{
			name:   "Gte",
			cond:   sqlbuild.Gte("pkey", []byte("a")),
			sql:    "pkey >= $1",
			params: []sqlbuild.Param{[]byte("a")},
		},
		{
			name:   "Lt",
			cond:   sqlbuild.Lt("pkey", []byte("z")),
			sql:    "pkey < $1",
			params: []sqlbuild.Param{[]byte("z")},
		},
		{
			name:   "In with two values",
			cond:   sqlbuild.In("pkey", "k1", "k2"),
			sql:    "pkey IN ($1,$2)",
			params: []sqlbuild.Param{"k1", "k2"},
		},
		{
			name:   "In with one value is not collapsed",
			cond:   sqlbuild.In("pkey", "k1"),
			sql:    "pkey IN ($1)",
			params: []sqlbuild.Param{"k1"},
		},
		{
			name:   "In with no values is always false",
			cond:   sqlbuild.In[string]("pkey"),
			sql:    "(1=0)",
			params: nil,
		},
		{
			name:   "And of two",
			cond:   sqlbuild.And(sqlbuild.Eq("ns", "n"), sqlbuild.Eq("pkey", "k")),
			sql:    "(ns = $1 AND pkey = $2)",
			params: []sqlbuild.Param{"n", "k"},
		},
		{
			name:   "And of three is flat",
			cond:   sqlbuild.And(sqlbuild.Eq("ns", "n"), sqlbuild.Gte("pkey", "a"), sqlbuild.Lt("pkey", "z")),
			sql:    "(ns = $1 AND pkey >= $2 AND pkey < $3)",
			params: []sqlbuild.Param{"n", "a", "z"},
		},
		{
			name:   "And of one keeps its parens",
			cond:   sqlbuild.And(sqlbuild.Eq("ns", "n")),
			sql:    "(ns = $1)",
			params: []sqlbuild.Param{"n"},
		},
		{
			name:   "And of none is always true",
			cond:   sqlbuild.And(),
			sql:    "(1=1)",
			params: nil,
		},
		{
			name: "nested And keeps both paren levels",
			cond: sqlbuild.And(
				sqlbuild.Eq("ns", "n"),
				sqlbuild.And(sqlbuild.Gte("pkey", "a"), sqlbuild.Lt("pkey", "z")),
			),
			sql:    "(ns = $1 AND (pkey >= $2 AND pkey < $3))",
			params: []sqlbuild.Param{"n", "a", "z"},
		},
		{
			name: "And skips nil children",
			cond: sqlbuild.And(nil, sqlbuild.Eq("ns", "n"), nil),
			sql:  "(ns = $1)", params: []sqlbuild.Param{"n"},
		},
		{
			name:   "In with no values nested in And keeps its parens",
			cond:   sqlbuild.And(sqlbuild.Eq("ns", "n"), sqlbuild.In[string]("pkey")),
			sql:    "(ns = $1 AND (1=0))",
			params: []sqlbuild.Param{"n"},
		},
		{
			name:   "In with values nested in And keeps its parens and numbers correctly",
			cond:   sqlbuild.And(sqlbuild.Eq("code", 1), sqlbuild.In("keys", "k1", "k2")),
			sql:    "(code = $1 AND keys IN ($2,$3))",
			params: []sqlbuild.Param{1, "k1", "k2"},
		},
		{
			name: "CondFunc writes a raw fragment",
			cond: sqlbuild.CondFunc(func(b *sqlbuild.Builder) {
				b.WriteString("pos=(SELECT max(pos) FROM status WHERE code!=").
					WriteParam(3).
					WriteString(")")
			}),
			sql:    "pos=(SELECT max(pos) FROM status WHERE code!=$1)",
			params: []sqlbuild.Param{3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sql, params := render(t, tc.cond)
			require.Equal(t, tc.sql, sql)
			require.Equal(t, tc.params, params)
		})
	}
}

func TestWriteWhere(t *testing.T) {
	t.Parallel()

	t.Run("no conditions writes nothing", func(t *testing.T) {
		t.Parallel()
		sql, params := sqlbuild.New().WriteString("SELECT * FROM t").WriteWhere().Build()
		require.Equal(t, "SELECT * FROM t", sql)
		require.Nil(t, params)
	})

	t.Run("all nil conditions writes nothing", func(t *testing.T) {
		t.Parallel()
		sql, _ := sqlbuild.New().WriteString("SELECT * FROM t").WriteWhere(nil, nil).Build()
		require.Equal(t, "SELECT * FROM t", sql)
	})

	t.Run("one condition", func(t *testing.T) {
		t.Parallel()
		sql, params := sqlbuild.New().
			WriteString("SELECT * FROM t").
			WriteWhere(sqlbuild.Eq("ns", "n")).
			Build()
		require.Equal(t, "SELECT * FROM t WHERE ns = $1", sql)
		require.Equal(t, []sqlbuild.Param{"n"}, params)
	})

	t.Run("two conditions are joined with AND and no extra parens", func(t *testing.T) {
		t.Parallel()
		// this is the shape squirrel produced for a base Where plus a keyset
		// cursor Where: WHERE code = $1 AND tx_id > $2
		sql, params := sqlbuild.New().
			WriteString("SELECT * FROM t").
			WriteWhere(sqlbuild.Eq("code", 1), sqlbuild.Gt("tx_id", "tx3")).
			Build()
		require.Equal(t, "SELECT * FROM t WHERE code = $1 AND tx_id > $2", sql)
		require.Equal(t, []sqlbuild.Param{1, "tx3"}, params)
	})
}
