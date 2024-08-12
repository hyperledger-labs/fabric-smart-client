/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUUIDLettersOnly(t *testing.T) {
	assert.Regexp(t, "^[a-zA-Z]{16}$", GenerateUUIDOnlyLetters())
}
