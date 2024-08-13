/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashUInt64(t *testing.T) {
	digest, err := HashUInt64([]byte("any_string?!"))
	assert.NoError(t, err)
	assert.Equal(t, uint64(12901064304492776662), digest)
}

func TestHashInt64(t *testing.T) {
	digest, err := HashInt64([]byte("any_string?!"))
	assert.NoError(t, err)
	assert.Equal(t, int64(6450532152246388331), digest)
}

func TestHashUInt32(t *testing.T) {
	digest, err := HashUInt32([]byte("any_string?!"))
	assert.NoError(t, err)
	assert.Equal(t, uint32(3003763105), digest)
}
