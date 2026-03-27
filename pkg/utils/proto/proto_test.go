/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package proto

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMarshalUnmarshal(t *testing.T) {
	msg := wrapperspb.String("hello world")
	b, err := Marshal(msg)
	require.NoError(t, err)

	got := &wrapperspb.StringValue{}
	require.NoError(t, Unmarshal(b, got))
	require.Equal(t, msg.Value, got.Value)
}

func TestMarshal_Empty(t *testing.T) {
	b, err := Marshal(&wrapperspb.StringValue{})
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestUnmarshal_InvalidData(t *testing.T) {
	// 0x0a = field 1 length-delimited, 0x80 0x01 = length 128, but no bytes follow
	err := Unmarshal([]byte{0x0a, 0x80, 0x01}, &wrapperspb.StringValue{})
	require.Error(t, err)
}

func TestEqual_True(t *testing.T) {
	a := wrapperspb.String("same")
	b := wrapperspb.String("same")
	require.True(t, Equal(a, b))
}

func TestEqual_False(t *testing.T) {
	a := wrapperspb.String("foo")
	b := wrapperspb.String("bar")
	require.False(t, Equal(a, b))
}

func TestClone(t *testing.T) {
	msg := wrapperspb.String("original")
	cloned, ok := Clone(msg).(*wrapperspb.StringValue)
	require.True(t, ok)
	require.True(t, Equal(msg, cloned))
	// Mutating the clone must not affect the original
	cloned.Value = "modified"
	require.Equal(t, "original", msg.Value)
}
