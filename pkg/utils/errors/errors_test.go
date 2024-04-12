/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package errors

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestWrapfSimpleNesting(t *testing.T) {
	nestedErr := errors.New("nested err")
	err := errors.Wrapf(nestedErr, "some error")
	assert.True(t, HasCause(err, nestedErr))
}

func TestWrapfDoubleNesting(t *testing.T) {
	nestedErr := errors.New("nested err")
	err := errors.Wrapf(errors.Wrapf(nestedErr, "some error"), "other error")
	assert.True(t, HasCause(err, nestedErr))
}
