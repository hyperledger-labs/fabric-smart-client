/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBetweenStrings_BothBounds(t *testing.T) { //nolint:paralleltest
	sql, args, err := betweenStrings("pkey", "a", "z").ToSql()
	require.NoError(t, err)
	require.Equal(t, "(pkey >= ? AND pkey < ?)", sql)
	require.Equal(t, []any{"a", "z"}, args)
}

func TestBetweenStrings_OnlyStart(t *testing.T) { //nolint:paralleltest
	sql, args, err := betweenStrings("pkey", "a", "").ToSql()
	require.NoError(t, err)
	require.Equal(t, "(pkey >= ?)", sql)
	require.Equal(t, []any{"a"}, args)
}

func TestBetweenStrings_OnlyEnd(t *testing.T) { //nolint:paralleltest
	sql, args, err := betweenStrings("pkey", "", "z").ToSql()
	require.NoError(t, err)
	require.Equal(t, "(pkey < ?)", sql)
	require.Equal(t, []any{"z"}, args)
}
