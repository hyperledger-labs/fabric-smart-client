/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keys_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/keys"
)

const (
	minUnicodeRuneValue   = 0            // U+0000
	maxUnicodeRuneValue   = utf8.MaxRune // U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
)

// createCompositeKey and its related functions and consts copied from core/chaincode/shim/chaincode.go
func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	var ck strings.Builder
	ck.WriteString(compositeKeyNamespace)
	ck.WriteString(objectType)
	fmt.Fprint(&ck, minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck.WriteString(att)
		fmt.Fprint(&ck, minUnicodeRuneValue)
	}
	return ck.String(), nil
}

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}

func TestValidateKey(t *testing.T) {
	t.Parallel()
	require.NoError(t, keys.ValidateKey("_key"))
	require.NoError(t, keys.ValidateKey("1lm7v0uzXp9p+Q/K4z0LM0bRWEAEi0qun3jTg8uNYrI="))
	key, err := createCompositeKey("token", []string{"thistype", "alice"})
	require.NoError(t, err)
	require.NoError(t, keys.ValidateKey(key))
	require.EqualError(t, keys.ValidateKey("_key?"), "key '_key?' is invalid")
	require.NoError(t, keys.ValidateKey("\x00"+string(utf8.MaxRune)+"initialized"))
	require.NoError(t, keys.ValidateKey("~tok~b9ae75c1c94e6389e543670fb5bb597553bcd6a4ae70d3ed0d0bf8822d10c793~0~"))
}

func TestValidateNamespace(t *testing.T) {
	t.Parallel()
	require.NoError(t, keys.ValidateNs("_token"))
	require.EqualError(t, keys.ValidateNs("+lifecycle"), "namespace '+lifecycle' is invalid")
}

func TestValidateKey_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		// Valid keys
		{"simple key", "simple_key", false},
		{"with dots", "key.with.dots", false},
		{"with dashes", "key-with-dashes", false},
		{"with tildes", "key~with~tildes", false},
		{"with slashes", "key/with/slashes", false},
		{"with plus", "key+with+plus", false},
		{"with equals", "key=with=equals", false},
		{"mixed case", "MixedCaseKey123", false},
		{"underscore prefix", "_underscore_prefix", false},
		{"numeric prefix", "0numeric_prefix", false},
		{"very long key", strings.Repeat("a", 1000), false},

		// Invalid keys - representative samples
		{"empty", "", true},
		{"with space", "key with space", true},
		{"with question", "key?", true},
		{"with exclamation", "key!", true},
		{"with at", "key@", true},
		{"with hash", "key#", true},
		{"with dollar", "key$", true},
		{"with percent", "key%", true},
		{"with caret", "key^", true},
		{"with ampersand", "key&", true},
		{"with asterisk", "key*", true},
		{"with parens", "key()", true},
		{"with brackets", "key[]", true},
		{"with braces", "key{}", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := keys.ValidateKey(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "is invalid")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateNs_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ns      string
		wantErr bool
	}{
		// Valid namespaces
		{"simple", "simple", false},
		{"with underscore", "with_underscore", false},
		{"with dash", "with-dash", false},
		{"with dot", "with.dot", false},
		{"mixed case", "MixedCase", false},
		{"numeric", "numeric123", false},
		{"underscore prefix", "_underscore_prefix", false},
		{"single char", "a", false},
		{"max length", strings.Repeat("a", 128), false},

		// Invalid namespaces - representative samples
		{"empty", "", true},
		{"too long", strings.Repeat("a", 129), true},
		{"with plus", "+lifecycle", true},
		{"with space", "with space", true},
		{"with exclamation", "with!", true},
		{"with at", "with@", true},
		{"with hash", "with#", true},
		{"with dollar", "with$", true},
		{"with slash", "with/", true},
		{"with tilde", "with~", true},
		{"with equals", "with=", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := keys.ValidateNs(tc.ns)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), "is invalid")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDummyVersionedIterator_Next(t *testing.T) {
	t.Parallel()

	items := []*driver.UnversionedRead{
		{Key: "key1", Raw: []byte("value1")},
		{Key: "key2", Raw: []byte("value2")},
		{Key: "key3", Raw: []byte("value3")},
	}

	t.Run("iterate through all items", func(t *testing.T) {
		t.Parallel()
		iter := &keys.DummyVersionedIterator{Items: items}

		for i, expected := range items {
			item, err := iter.Next()
			require.NoError(t, err)
			require.NotNil(t, item, "item %d should not be nil", i)
			require.Equal(t, expected.Key, item.Key)
			require.Equal(t, expected.Raw, item.Raw)
		}

		// End of iteration
		item, err := iter.Next()
		require.NoError(t, err)
		require.Nil(t, item)

		// Subsequent calls return nil
		item, err = iter.Next()
		require.NoError(t, err)
		require.Nil(t, item)
	})

	t.Run("empty and nil items", func(t *testing.T) {
		cases := []struct {
			name  string
			items []*driver.UnversionedRead
		}{
			{"empty slice", []*driver.UnversionedRead{}},
			{"nil slice", nil},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				iter := &keys.DummyVersionedIterator{Items: tc.items}
				item, err := iter.Next()
				require.NoError(t, err)
				require.Nil(t, item)
			})
		}
	})
}

func TestDummyVersionedIterator_Close(t *testing.T) {
	t.Parallel()

	items := []*driver.UnversionedRead{{Key: "key1", Raw: []byte("value1")}}
	iter := &keys.DummyVersionedIterator{Items: items}

	// Close should not panic
	require.NotPanics(t, func() {
		iter.Close()
	})

	// Operations work after close (Close is a no-op)
	item, err := iter.Next()
	require.NoError(t, err)
	require.NotNil(t, item)
}

func TestNamespaceSeparator(t *testing.T) {
	t.Parallel()

	require.Equal(t, "\u0000", keys.NamespaceSeparator)
	require.Equal(t, "\x00", keys.NamespaceSeparator)
}
