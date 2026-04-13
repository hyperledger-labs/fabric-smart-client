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
