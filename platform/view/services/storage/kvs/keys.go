/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
)

// CreateCompositeKey builds a composite key from objectType and attributes, using the
// encoding of the Fabric chaincode shim. See compose.CreateCompositeKey.
func CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return compose.CreateCompositeKey(&strings.Builder{}, objectType, attributes...)
}

// CreateCompositeKeyOrPanic is like CreateCompositeKey but panics if objectType or an
// attribute is invalid.
func CreateCompositeKeyOrPanic(objectType string, attributes []string) string {
	return compose.CreateCompositeKeyOrPanic(&strings.Builder{}, objectType, attributes...)
}

// CreateRangeKeysForPartialCompositeKey returns the start and end keys of a range scan
// over all composite keys that begin with objectType and attributes.
func CreateRangeKeysForPartialCompositeKey(objectType string, attributes []string) (startKey, endKey string, err error) {
	return compose.CreateRangeKeysForPartialCompositeKey(objectType, attributes...)
}

// SplitCompositeKey splits a composite key built by CreateCompositeKey into its
// objectType and attributes. It returns an error if compositeKey is not such a key.
func SplitCompositeKey(compositeKey string) (string, []string, error) {
	return compose.SplitCompositeKey(compositeKey)
}
