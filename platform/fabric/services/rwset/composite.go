/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
)

// CreateCompositeKey builds a composite key from objectType and attributes, using the
// encoding of the Fabric chaincode shim. See compose.CreateCompositeKey.
func CreateCompositeKey(objectType string, attributes []string) (string, error) {
	return compose.CreateCompositeKey(&strings.Builder{}, objectType, attributes...)
}

// CreateRangeKeysForPartialCompositeKey returns the start and end keys of a range scan
// over all composite keys that begin with objectType and attributes.
func CreateRangeKeysForPartialCompositeKey(objectType string, attributes []string) (startKey, endKey string, err error) {
	return compose.CreateRangeKeysForPartialCompositeKey(objectType, attributes...)
}

// SplitCompositeKey splits a composite key into its objectType and attributes.
// A key that is not a composite key is returned unchanged as the objectType, with no
// attributes and no error, so callers can derive keys from plain and composite keys alike.
func SplitCompositeKey(compositeKey string) (string, []string, error) {
	if !compose.IsCompositeKey(compositeKey) || strings.IndexByte(compositeKey[1:], 0) < 0 {
		return compositeKey, nil, nil
	}
	objectType, attrs, err := compose.SplitCompositeKey(compositeKey)
	if err != nil {
		return "", nil, err
	}
	if len(attrs) == 0 {
		attrs = nil
	}
	return objectType, attrs, nil
}
