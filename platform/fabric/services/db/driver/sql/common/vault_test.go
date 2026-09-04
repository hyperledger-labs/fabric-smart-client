/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/internal/storage/sqlbuild"
)

func renderCondition(c sqlbuild.Condition) (string, []sqlbuild.Param) {
	b := sqlbuild.New()
	c.WriteTo(b)
	return b.Build()
}

func TestBetweenStrings_BothBounds(t *testing.T) { //nolint:paralleltest
	sql, args := renderCondition(betweenStrings("pkey", "a", "z"))
	require.Equal(t, "(pkey >= $1 AND pkey < $2)", sql)
	require.Equal(t, []sqlbuild.Param{"a", "z"}, args)
}

func TestBetweenStrings_OnlyStart(t *testing.T) { //nolint:paralleltest
	sql, args := renderCondition(betweenStrings("pkey", "a", ""))
	require.Equal(t, "(pkey >= $1)", sql)
	require.Equal(t, []sqlbuild.Param{"a"}, args)
}

func TestBetweenStrings_OnlyEnd(t *testing.T) { //nolint:paralleltest
	sql, args := renderCondition(betweenStrings("pkey", "", "z"))
	require.Equal(t, "(pkey < $1)", sql)
	require.Equal(t, []sqlbuild.Param{"z"}, args)
}

// GetStateRange(ctx, ns, "", "") relies on this rendering as a tautology
// rather than as an empty string.
func TestBetweenStrings_NoBounds(t *testing.T) { //nolint:paralleltest
	sql, args := renderCondition(betweenStrings("pkey", "", ""))
	require.Equal(t, "(1=1)", sql)
	require.Nil(t, args)
}
