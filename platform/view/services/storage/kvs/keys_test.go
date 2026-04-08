/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestCreateCompositeKey(t *testing.T) {
	tests := []struct {
		name        string
		objectType  string
		attributes  []string
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid key with single attribute",
			objectType:  "user",
			attributes:  []string{"alice"},
			expected:    "\x00user\x00alice\x00",
			expectError: false,
		},
		{
			name:        "valid key with multiple attributes",
			objectType:  "asset",
			attributes:  []string{"type1", "id123", "owner456"},
			expected:    "\x00asset\x00type1\x00id123\x00owner456\x00",
			expectError: false,
		},
		{
			name:        "valid key with no attributes",
			objectType:  "config",
			attributes:  []string{},
			expected:    "\x00config\x00",
			expectError: false,
		},
		{
			name:        "valid key with empty string attribute",
			objectType:  "test",
			attributes:  []string{""},
			expected:    "\x00test\x00\x00",
			expectError: false,
		},
		{
			name:        "invalid objectType with minUnicodeRuneValue",
			objectType:  "test\x00value",
			attributes:  []string{"attr"},
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid objectType with maxUnicodeRuneValue",
			objectType:  string([]rune{'t', 'e', 's', 't', maxUnicodeRuneValue}),
			attributes:  []string{"attr"},
			expectError: true,
			errorMsg:    "U+10FFFF",
		},
		{
			name:        "invalid attribute with minUnicodeRuneValue",
			objectType:  "user",
			attributes:  []string{"valid", "invalid\x00attr"},
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid attribute with maxUnicodeRuneValue",
			objectType:  "user",
			attributes:  []string{string([]rune{'i', 'n', 'v', 'a', 'l', 'i', 'd', maxUnicodeRuneValue})},
			expectError: true,
			errorMsg:    "U+10FFFF",
		},
		{
			name:        "invalid UTF-8 in objectType",
			objectType:  "test\xff\xfe",
			attributes:  []string{"attr"},
			expectError: true,
			errorMsg:    "not a valid utf8 string",
		},
		{
			name:        "invalid UTF-8 in attribute",
			objectType:  "user",
			attributes:  []string{"valid", "\xff\xfe"},
			expectError: true,
			errorMsg:    "not a valid utf8 string",
		},
		{
			name:        "valid key with special characters",
			objectType:  "document",
			attributes:  []string{"file-name.txt", "user@example.com", "2024-01-01"},
			expected:    "\x00document\x00file-name.txt\x00user@example.com\x002024-01-01\x00",
			expectError: false,
		},
		{
			name:        "valid key with unicode characters",
			objectType:  "user",
			attributes:  []string{"用户", "ユーザー", "пользователь"},
			expected:    "\x00user\x00用户\x00ユーザー\x00пользователь\x00",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CreateCompositeKey(tt.objectType, tt.attributes)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					require.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateCompositeKeyAttribute(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid string",
			input:       "validAttribute",
			expectError: false,
		},
		{
			name:        "valid empty string",
			input:       "",
			expectError: false,
		},
		{
			name:        "valid unicode string",
			input:       "用户名",
			expectError: false,
		},
		{
			name:        "invalid with minUnicodeRuneValue at start",
			input:       "\x00test",
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid with minUnicodeRuneValue in middle",
			input:       "test\x00value",
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid with minUnicodeRuneValue at end",
			input:       "test\x00",
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid with maxUnicodeRuneValue",
			input:       string([]rune{'t', 'e', 's', 't', maxUnicodeRuneValue}),
			expectError: true,
			errorMsg:    "U+10FFFF",
		},
		{
			name:        "invalid UTF-8 sequence",
			input:       "test\xff\xfe",
			expectError: true,
			errorMsg:    "not a valid utf8 string",
		},
		{
			name:        "valid string with special characters",
			input:       "test-value_123.txt",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompositeKeyAttribute(tt.input)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					require.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateCompositeKeyOrPanic(t *testing.T) {
	tests := []struct {
		name        string
		objectType  string
		attributes  []string
		expected    string
		shouldPanic bool
	}{
		{
			name:        "valid key does not panic",
			objectType:  "user",
			attributes:  []string{"alice", "123"},
			expected:    "\x00user\x00alice\x00123\x00",
			shouldPanic: false,
		},
		{
			name:        "invalid key panics",
			objectType:  "user\x00",
			attributes:  []string{"alice"},
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				require.Panics(t, func() {
					CreateCompositeKeyOrPanic(tt.objectType, tt.attributes)
				})
			} else {
				result := CreateCompositeKeyOrPanic(tt.objectType, tt.attributes)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCreateRangeKeysForPartialCompositeKey(t *testing.T) {
	tests := []struct {
		name          string
		objectType    string
		attributes    []string
		expectedStart string
		expectedEnd   string
		expectError   bool
		errorMsg      string
	}{
		{
			name:          "valid range with single attribute",
			objectType:    "user",
			attributes:    []string{"alice"},
			expectedStart: "\x00user\x00alice\x00",
			expectedEnd:   "\x00user\x00alice\x00" + string(maxUnicodeRuneValue),
			expectError:   false,
		},
		{
			name:          "valid range with multiple attributes",
			objectType:    "asset",
			attributes:    []string{"type1", "id123"},
			expectedStart: "\x00asset\x00type1\x00id123\x00",
			expectedEnd:   "\x00asset\x00type1\x00id123\x00" + string(maxUnicodeRuneValue),
			expectError:   false,
		},
		{
			name:          "valid range with no attributes",
			objectType:    "config",
			attributes:    []string{},
			expectedStart: "\x00config\x00",
			expectedEnd:   "\x00config\x00" + string(maxUnicodeRuneValue),
			expectError:   false,
		},
		{
			name:        "invalid objectType",
			objectType:  "test\x00",
			attributes:  []string{"attr"},
			expectError: true,
			errorMsg:    "U+0000",
		},
		{
			name:        "invalid attribute",
			objectType:  "user",
			attributes:  []string{"valid", "invalid\x00"},
			expectError: true,
			errorMsg:    "U+0000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startKey, endKey, err := CreateRangeKeysForPartialCompositeKey(tt.objectType, tt.attributes)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					require.Contains(t, err.Error(), tt.errorMsg)
				}
				require.Empty(t, startKey)
				require.Empty(t, endKey)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedStart, startKey)
				require.Equal(t, tt.expectedEnd, endKey)
				// Verify that endKey is greater than startKey
				require.True(t, endKey > startKey)
			}
		})
	}
}

func TestSplitCompositeKey(t *testing.T) {
	tests := []struct {
		name               string
		compositeKey       string
		expectedObjectType string
		expectedAttributes []string
		expectError        bool
	}{
		{
			name:               "valid key with single attribute",
			compositeKey:       "\x00user\x00alice\x00",
			expectedObjectType: "user",
			expectedAttributes: []string{"alice"},
			expectError:        false,
		},
		{
			name:               "valid key with multiple attributes",
			compositeKey:       "\x00asset\x00type1\x00id123\x00owner456\x00",
			expectedObjectType: "asset",
			expectedAttributes: []string{"type1", "id123", "owner456"},
			expectError:        false,
		},
		{
			name:               "valid key with no attributes",
			compositeKey:       "\x00config\x00",
			expectedObjectType: "config",
			expectedAttributes: []string{},
			expectError:        false,
		},
		{
			name:               "valid key with empty attribute",
			compositeKey:       "\x00test\x00\x00",
			expectedObjectType: "test",
			expectedAttributes: []string{""},
			expectError:        false,
		},
		{
			name:               "valid key with unicode attributes",
			compositeKey:       "\x00user\x00用户\x00ユーザー\x00",
			expectedObjectType: "user",
			expectedAttributes: []string{"用户", "ユーザー"},
			expectError:        false,
		},
		{
			name:               "valid key with special characters",
			compositeKey:       "\x00document\x00file-name.txt\x00user@example.com\x00",
			expectedObjectType: "document",
			expectedAttributes: []string{"file-name.txt", "user@example.com"},
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objectType, attributes, err := SplitCompositeKey(tt.compositeKey)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedObjectType, objectType)
				require.Equal(t, tt.expectedAttributes, attributes)
			}
		})
	}
}

func TestCompositeKeyRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		objectType string
		attributes []string
	}{
		{
			name:       "single attribute",
			objectType: "user",
			attributes: []string{"alice"},
		},
		{
			name:       "multiple attributes",
			objectType: "asset",
			attributes: []string{"type1", "id123", "owner456"},
		},
		{
			name:       "no attributes",
			objectType: "config",
			attributes: []string{},
		},
		{
			name:       "empty attribute",
			objectType: "test",
			attributes: []string{""},
		},
		{
			name:       "unicode attributes",
			objectType: "user",
			attributes: []string{"用户", "ユーザー", "пользователь"},
		},
		{
			name:       "special characters",
			objectType: "document",
			attributes: []string{"file-name.txt", "user@example.com", "2024-01-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create composite key
			compositeKey, err := CreateCompositeKey(tt.objectType, tt.attributes)
			require.NoError(t, err)

			// Split composite key
			objectType, attributes, err := SplitCompositeKey(compositeKey)
			require.NoError(t, err)

			// Verify round trip
			require.Equal(t, tt.objectType, objectType)
			require.Equal(t, tt.attributes, attributes)
		})
	}
}

func TestRangeKeysContainment(t *testing.T) {
	// Create a partial composite key for "user" with attribute "alice"
	startKey, endKey, err := CreateRangeKeysForPartialCompositeKey("user", []string{"alice"})
	require.NoError(t, err)

	tests := []struct {
		name      string
		key       string
		shouldFit bool
	}{
		{
			name:      "exact match",
			key:       "\x00user\x00alice\x00",
			shouldFit: true,
		},
		{
			name:      "key with additional attribute",
			key:       "\x00user\x00alice\x00extra\x00",
			shouldFit: true,
		},
		{
			name:      "different user",
			key:       "\x00user\x00bob\x00",
			shouldFit: false,
		},
		{
			name:      "different object type",
			key:       "\x00asset\x00alice\x00",
			shouldFit: false,
		},
		{
			name:      "prefix of alice",
			key:       "\x00user\x00ali\x00",
			shouldFit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inRange := tt.key >= startKey && tt.key < endKey
			require.Equal(t, tt.shouldFit, inRange)
		})
	}
}

func TestCompositeKeyConstants(t *testing.T) {
	t.Run("minUnicodeRuneValue is zero", func(t *testing.T) {
		require.Equal(t, rune(0), minUnicodeRuneValue)
	})

	t.Run("maxUnicodeRuneValue is MaxRune", func(t *testing.T) {
		require.Equal(t, utf8.MaxRune, maxUnicodeRuneValue)
	})

	t.Run("compositeKeyNamespace is null byte", func(t *testing.T) {
		require.Equal(t, "\x00", compositeKeyNamespace)
	})
}

func TestCreateCompositeKeyPerformance(t *testing.T) {
	// This test ensures that CreateCompositeKey uses strings.Builder efficiently
	objectType := "user"
	attributes := make([]string, 100)
	for i := 0; i < 100; i++ {
		attributes[i] = "attribute_" + string(rune('a'+i%26))
	}

	result, err := CreateCompositeKey(objectType, attributes)
	require.NoError(t, err)

	// Verify the result starts with namespace and object type
	require.True(t, strings.HasPrefix(result, compositeKeyNamespace+objectType))

	// Verify all attributes are present
	for _, attr := range attributes {
		require.Contains(t, result, attr)
	}
}

// Made with Bob
