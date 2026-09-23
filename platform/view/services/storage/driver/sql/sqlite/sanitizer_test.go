/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSanitizerIsIdentity checks sqlite needs no escaping: both directions return the input.
func TestSanitizerIsIdentity(t *testing.T) {
	t.Parallel()

	s := NewSanitizer()
	in := "table'; DROP TABLE x; --"

	enc, err := s.Encode(in)
	require.NoError(t, err)
	assert.Equal(t, in, enc)

	dec, err := s.Decode(in)
	require.NoError(t, err)
	assert.Equal(t, in, dec)
}
