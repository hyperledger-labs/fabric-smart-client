/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapfSimpleNesting(t *testing.T) {
	nestedErr := New("nested err")
	err := Wrapf(nestedErr, "some error")
	assert.True(t, HasCause(err, nestedErr))
}

func TestWrapfDoubleNesting(t *testing.T) {
	nestedErr := Errorf("nested err")
	err := Wrapf(Wrapf(nestedErr, "some error"), "other error")
	assert.True(t, HasCause(err, nestedErr))
}

func TestHasCauseNil(t *testing.T) {
	assert.False(t, HasCause(New("some err"), nil))
}

type newErrType struct{ msg string }

func (e *newErrType) Error() string { return e.msg }

func TestHasType(t *testing.T) {
	nestedErr := &newErrType{msg: "nested err"}
	err := Wrapf(nestedErr, "some error")
	assert.True(t, HasType(err, &newErrType{}))
}

func TestHasTypeNil(t *testing.T) {
	assert.False(t, HasType(New("some err"), nil))
}
