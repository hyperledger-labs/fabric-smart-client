/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"strings"
	"unicode/utf8"

	"github.com/pkg/errors"
)

const (
	minUnicodeRuneValue   rune = 0            // U+0000
	maxUnicodeRuneValue   rune = utf8.MaxRune // U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace      = "\x00"
)

// CreateCompositeKey and its related functions and consts copied from core/chaincode/shim/chaincode.go
func CreateCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString(compositeKeyNamespace)
	sb.WriteString(objectType)
	sb.WriteRune(rune(minUnicodeRuneValue))
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		sb.WriteString(att)
		sb.WriteRune(minUnicodeRuneValue)
	}
	return sb.String(), nil
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

func CreateCompositeKeyOrPanic(objectType string, attributes []string) string {
	k, err := CreateCompositeKey(objectType, attributes)
	if err != nil {
		panic(err)
	}
	return k
}

func CreateRangeKeysForPartialCompositeKey(objectType string, attributes []string) (string, string, error) {
	partialCompositeKey, err := CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", "", err
	}
	startKey := partialCompositeKey
	endKey := partialCompositeKey + string(maxUnicodeRuneValue)

	return startKey, endKey, nil
}

// SplitCompositeKey splits the passed composite key into objectType and attributes
func SplitCompositeKey(compositeKey string) (string, []string, error) {
	componentIndex := 1
	var components []string
	for i := 1; i < len(compositeKey); i++ {
		if rune(compositeKey[i]) == minUnicodeRuneValue {
			components = append(components, compositeKey[componentIndex:i])
			componentIndex = i + 1
		}
	}
	return components[0], components[1:], nil
}
