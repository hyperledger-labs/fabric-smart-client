/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrapfSimpleNesting(t *testing.T) {
	nestedErr := New("nested err")
	err := Wrapf(nestedErr, "some error")
	require.True(t, HasCause(err, nestedErr))
}

func TestWrapfDoubleNesting(t *testing.T) {
	nestedErr := Errorf("nested err")
	err := Wrapf(Wrapf(nestedErr, "some error"), "other error")
	require.True(t, HasCause(err, nestedErr))
}

func TestHasCauseNil(t *testing.T) {
	require.False(t, HasCause(New("some err"), nil))
}

type newErrType struct{ msg string }

func (e *newErrType) Error() string { return e.msg }

func TestHasType(t *testing.T) {
	nestedErr := &newErrType{msg: "nested err"}
	err := Wrapf(nestedErr, "some error")
	require.True(t, HasType(err, &newErrType{}))
}

func TestHasTypeNil(t *testing.T) {
	require.False(t, HasType(New("some err"), nil))
}

func TestNew(t *testing.T) {
	err := New("test error")
	require.Error(t, err)
	require.Equal(t, "test error", err.Error())
}

func TestErrorf(t *testing.T) {
	err := Errorf("error %d", 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error 42")
}

func TestWrap(t *testing.T) {
	inner := New("inner")
	wrapped := Wrap(inner, "outer")
	require.True(t, HasCause(wrapped, inner))
}

func TestWithMessage(t *testing.T) {
	inner := New("inner")
	annotated := WithMessage(inner, "some message")
	require.True(t, HasCause(annotated, inner))
}

func TestWithMessagef(t *testing.T) {
	inner := New("inner")
	annotated := WithMessagef(inner, "message %d", 1)
	require.True(t, HasCause(annotated, inner))
}

func TestWithStack(t *testing.T) {
	inner := New("inner")
	stacked := WithStack(inner)
	require.True(t, HasCause(stacked, inner))
}

func TestCause(t *testing.T) {
	inner := New("inner")
	wrapped := Wrapf(inner, "outer")
	cause := Cause(wrapped)
	require.NotNil(t, cause)
}

func TestIs(t *testing.T) {
	inner := New("inner")
	wrapped := Wrapf(inner, "outer")
	require.True(t, Is(wrapped, inner))
}

func TestIs_Nil(t *testing.T) {
	require.False(t, Is(New("err"), nil))
}

func TestJoin(t *testing.T) {
	e1 := New("error 1")
	e2 := New("error 2")
	joined := Join(e1, e2)
	require.Error(t, joined)
	require.True(t, Is(joined, e1))
	require.True(t, Is(joined, e2))
}

func TestJoin_NilErrors(t *testing.T) {
	result := Join(nil, nil)
	require.NoError(t, result)
}
