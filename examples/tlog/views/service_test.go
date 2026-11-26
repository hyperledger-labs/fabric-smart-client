/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSize(t *testing.T) {
	t.Parallel()

	n, err := parseSize(nil)
	require.NoError(t, err)
	require.Zero(t, n)

	n, err = parseSize([]byte("9223372036854775807")) // math.MaxInt64
	require.NoError(t, err)
	require.Equal(t, uint64(1<<63-1), n)

	_, err = parseSize([]byte("9223372036854775808")) // math.MaxInt64 + 1
	require.ErrorContains(t, err, "exceeds the int64 range")

	_, err = parseSize([]byte("-1"))
	require.ErrorContains(t, err, "invalid log size")
}
