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

func TestMarshallMetadata_RoundTrip(t *testing.T) { //nolint:paralleltest
	// gob decodes an empty []byte back as nil, so don't assert a round-trip
	// on one — the callers only ever store non-empty metadata values.
	in := map[string][]byte{"a": []byte("one"), "b": []byte("two")}

	raw, err := marshallMetadata(in)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	out, err := unmarshalMetadata(raw)
	require.NoError(t, err)
	require.Equal(t, in, out)
}

func TestMarshallMetadata_Empty(t *testing.T) { //nolint:paralleltest
	raw, err := marshallMetadata(map[string][]byte{})
	require.NoError(t, err)

	out, err := unmarshalMetadata(raw)
	require.NoError(t, err)
	require.Empty(t, out)
}

// GetStateMetadata stores an empty metadata column for states written without
// metadata, so unmarshalling nothing must not be an error.
func TestUnmarshalMetadata_NoInput(t *testing.T) { //nolint:paralleltest
	out, err := unmarshalMetadata(nil)
	require.NoError(t, err)
	require.Nil(t, out)

	out, err = unmarshalMetadata([]byte{})
	require.NoError(t, err)
	require.Nil(t, out)
}

func TestUnmarshalMetadata_Garbage(t *testing.T) { //nolint:paralleltest
	_, err := unmarshalMetadata([]byte{0xff, 0x00, 0x42})
	require.Error(t, err)
}
