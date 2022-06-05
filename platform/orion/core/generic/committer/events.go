/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"github.com/pkg/errors"
	"strings"
	"unicode/utf8"
)

const (
	minUnicodeRuneValue   rune = 0            // U+0000
	maxUnicodeRuneValue   rune = utf8.MaxRune // U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace      = "\x00"
)

// TxEvent contains information for token transaction commit
type TxEvent struct {
	Txid           string
	DependantTxIDs []string
	Committed      bool
	Block          uint64
	IndexInBlock   int
	CommitPeer     string
	Err            error
}

// CreateCompositeKey creates a composite key in the given string builder.
func CreateCompositeKey(sb *strings.Builder, objectType string, attributes ...string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
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

// CreateCompositeKeyOrPanic creates a composite key in the given string builder.
// It panics if the attributes are invalid.
func CreateCompositeKeyOrPanic(sb *strings.Builder, objectType string, attributes ...string) string {
	k, err := CreateCompositeKey(sb, objectType, attributes...)
	if err != nil {
		panic(err)
	}
	return k
}

// AppendAttributes appends the attributes to the end of the given string builder.
func AppendAttributes(sb *strings.Builder, attributes ...string) (string, error) {
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		sb.WriteString(att)
		sb.WriteRune(minUnicodeRuneValue)
	}
	return sb.String(), nil
}

// AppendAttributesOrPanic appends the attributes to the end of the given string builder.
// It panics if the attributes are invalid.
func AppendAttributesOrPanic(sb *strings.Builder, attributes ...string) string {
	k, err := AppendAttributes(sb, attributes...)
	if err != nil {
		panic(err)
	}
	return k
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
