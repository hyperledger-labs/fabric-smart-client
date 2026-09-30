/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendAttributes(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	CreateCompositeKeyOrPanic(&sb, "ot", "1", "2")
	k := AppendAttributesOrPanic(&sb, "3")
	require.Equal(t, CreateCompositeKeyOrPanic(&strings.Builder{}, "ot", "1", "2", "3"), k)
}

func TestCreateCompositeKey(t *testing.T) {
	t.Parallel()
	sb := &strings.Builder{}
	key, err := CreateCompositeKey(sb, "myType", "attr1", "attr2")
	require.NoError(t, err)
	require.NotEmpty(t, key)
}

func TestCreateCompositeKey_InvalidUTF8(t *testing.T) {
	t.Parallel()
	sb := &strings.Builder{}
	_, err := CreateCompositeKey(sb, "myType", string([]byte{0xFF, 0xFE}))
	require.Error(t, err)
}

func TestCreateCompositeKey_ForbiddenMinRune(t *testing.T) {
	t.Parallel()
	sb := &strings.Builder{}
	_, err := CreateCompositeKey(sb, "myType", string(rune(0)))
	require.Error(t, err)
}

func TestCreateCompositeKey_ForbiddenMaxRune(t *testing.T) {
	t.Parallel()
	sb := &strings.Builder{}
	_, err := CreateCompositeKey(sb, "myType", string(rune(0x10FFFF)))
	require.Error(t, err)
}

func TestCreateCompositeKeyOrPanic_Panics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() {
		CreateCompositeKeyOrPanic(&strings.Builder{}, string(rune(0)))
	})
}

func TestAppendAttributes_InvalidUTF8(t *testing.T) {
	t.Parallel()
	sb := &strings.Builder{}
	_, err := AppendAttributes(sb, string([]byte{0xFF}))
	require.Error(t, err)
}

func TestAppendAttributesOrPanic_Panics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() {
		AppendAttributesOrPanic(&strings.Builder{}, string(rune(0)))
	})
}

func TestCreateTxTopic_WithTxID(t *testing.T) {
	t.Parallel()
	sb, key := CreateTxTopic("net", "chan", "txid")
	require.NotNil(t, sb)
	require.NotEmpty(t, key)
}

func TestCreateTxTopic_WithoutTxID(t *testing.T) {
	t.Parallel()
	_, keyNoTx := CreateTxTopic("net", "chan", "")
	_, keyWithTx := CreateTxTopic("net", "chan", "txid")
	require.NotEmpty(t, keyNoTx)
	require.NotEqual(t, keyNoTx, keyWithTx)
}

func TestCreateRangeKeysForPartialCompositeKey(t *testing.T) {
	t.Parallel()
	start, end, err := CreateRangeKeysForPartialCompositeKey("ot", "1")
	require.NoError(t, err)
	require.Equal(t, "\x00ot\x001\x00", start)
	require.Equal(t, start+"\U0010FFFF", end)

	inside := CreateCompositeKeyOrPanic(&strings.Builder{}, "ot", "1", "zzz")
	outside := CreateCompositeKeyOrPanic(&strings.Builder{}, "ot", "2")
	require.True(t, start <= inside && inside < end)
	require.False(t, start <= outside && outside < end)

	_, _, err = CreateRangeKeysForPartialCompositeKey("ot\x00")
	require.ErrorContains(t, err, "U+0000")
}

func TestSplitCompositeKey(t *testing.T) {
	t.Parallel()
	objectType, attrs, err := SplitCompositeKey(CreateCompositeKeyOrPanic(&strings.Builder{}, "ot", "1", "2"))
	require.NoError(t, err)
	require.Equal(t, "ot", objectType)
	require.Equal(t, []string{"1", "2"}, attrs)

	objectType, attrs, err = SplitCompositeKey(CreateCompositeKeyOrPanic(&strings.Builder{}, "ot"))
	require.NoError(t, err)
	require.Equal(t, "ot", objectType)
	require.Empty(t, attrs)

	for _, k := range []string{"", "plain", "\x00", "\x00ot"} {
		_, _, err = SplitCompositeKey(k)
		require.ErrorContains(t, err, "not a composite key", "key %q", k)
		require.Equal(t, k != "" && k[0] == 0, IsCompositeKey(k), "key %q", k)
	}
}
