/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeccak256KnownVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		// Well-known Ethereum keccak-256 vectors.
		{name: "empty", in: "", want: "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{name: "abc", in: "abc", want: "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, hex.EncodeToString(Keccak256([]byte(tc.in))))
		})
	}
}

// TestKeccak256Concatenation guards the variadic behaviour: hashing several slices must equal
// hashing their concatenation, because callers rely on that when building composite keys.
func TestKeccak256Concatenation(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Keccak256([]byte("abc")), Keccak256([]byte("a"), []byte("bc")))
	assert.Equal(t, Keccak256([]byte("abc")), Keccak256([]byte("ab"), []byte("c")))
}

func TestKeccak256HashLength(t *testing.T) {
	t.Parallel()

	h := Keccak256Hash([]byte("anything"))
	assert.Len(t, h[:], HashLength)
	assert.Equal(t, Keccak256([]byte("anything")), h[:])
}
