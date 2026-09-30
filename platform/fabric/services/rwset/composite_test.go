/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestCreateCompositeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		objectType  string
		attributes  []string
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:       "valid key with multiple attributes",
			objectType: "asset",
			attributes: []string{"type", "id", "owner"},
			expected:   "\x00asset\x00type\x00id\x00owner\x00",
		},
		{
			name:       "valid key with no attributes",
			objectType: "config",
			attributes: nil,
			expected:   "\x00config\x00",
		},
		{
			name:        "invalid object type contains min rune",
			objectType:  "asset\x00type",
			attributes:  []string{"id"},
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid attribute contains max rune",
			objectType:  "asset",
			attributes:  []string{"id", string([]rune{'a', utf8.MaxRune})},
			expectError: true,
			errorMsg:    "U+10FFFF",
		},
		{
			name:        "invalid utf8 attribute",
			objectType:  "asset",
			attributes:  []string{"\xff\xfe"},
			expectError: true,
			errorMsg:    "not a valid utf8 string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, err := CreateCompositeKey(tt.objectType, tt.attributes)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, key)
		})
	}
}

func TestCreateRangeKeysForPartialCompositeKey(t *testing.T) {
	t.Parallel()

	t.Run("valid partial key range", func(t *testing.T) {
		t.Parallel()
		start, end, err := CreateRangeKeysForPartialCompositeKey("asset", []string{"type", "id"})
		require.NoError(t, err)
		require.Equal(t, "\x00asset\x00type\x00id\x00", start)
		require.Equal(t, start+"\U0010FFFF", end)
	})

	t.Run("invalid partial key range", func(t *testing.T) {
		t.Parallel()
		_, _, err := CreateRangeKeysForPartialCompositeKey("asset\x00", []string{"id"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "U+0000")
	})
}

func TestSplitCompositeKey(t *testing.T) {
	t.Parallel()

	t.Run("split key with attributes", func(t *testing.T) {
		t.Parallel()
		key, err := CreateCompositeKey("asset", []string{"type", "id"})
		require.NoError(t, err)

		objectType, attrs, err := SplitCompositeKey(key)
		require.NoError(t, err)
		require.Equal(t, "asset", objectType)
		require.Equal(t, []string{"type", "id"}, attrs)
	})

	t.Run("split key without attributes", func(t *testing.T) {
		t.Parallel()
		key, err := CreateCompositeKey("config", nil)
		require.NoError(t, err)

		objectType, attrs, err := SplitCompositeKey(key)
		require.NoError(t, err)
		require.Equal(t, "config", objectType)
		require.Nil(t, attrs)
	})

	t.Run("plain key", func(t *testing.T) {
		t.Parallel()
		objectType, attrs, err := SplitCompositeKey("plain-key")
		require.NoError(t, err)
		require.Equal(t, "plain-key", objectType)
		require.Nil(t, attrs)
	})
}
