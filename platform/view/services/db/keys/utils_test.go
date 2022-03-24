/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package keys_test

import (
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/keys"
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
	ck := compositeKeyNamespace + objectType + fmt.Sprint(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + fmt.Sprint(minUnicodeRuneValue)
	}
	return ck, nil
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
	assert.NoError(t, keys.ValidateKey("_key"))
	assert.NoError(t, keys.ValidateKey("1lm7v0uzXp9p+Q/K4z0LM0bRWEAEi0qun3jTg8uNYrI="))
	key, err := createCompositeKey("token", []string{"thistype", "alice"})
	assert.NoError(t, err)
	assert.NoError(t, keys.ValidateKey(key))
	assert.EqualError(t, keys.ValidateKey("_key?"), "key '_key?' is invalid")
	assert.NoError(t, keys.ValidateKey("\x00"+string(utf8.MaxRune)+"initialized"))
	assert.NoError(t, keys.ValidateKey("~tok~b9ae75c1c94e6389e543670fb5bb597553bcd6a4ae70d3ed0d0bf8822d10c793~0~"))
}

func TestValidateNamespace(t *testing.T) {
	assert.NoError(t, keys.ValidateNs("_token"))
	assert.EqualError(t, keys.ValidateNs("+lifecycle"), "namespace '+lifecycle' is invalid")
}
