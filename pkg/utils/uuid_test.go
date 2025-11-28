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

func BenchmarkUUID(b *testing.B) {
	b.Run("GenerateBytesUUID", func(b *testing.B) {
		for b.Loop() {
			_ = GenerateBytesUUID()
		}
	})

	b.Run("GenerateUUID", func(b *testing.B) {
		for b.Loop() {
			_ = GenerateUUID()
		}
	})

	b.Run("idBytesToStr", func(b *testing.B) {
		k := GenerateBytesUUID()
		for b.Loop() {
			_ = idBytesToStr(k)
		}
	})
}
