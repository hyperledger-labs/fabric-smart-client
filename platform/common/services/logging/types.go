/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
)

// Keys logs lazily the keys of a map
func Keys[K comparable, V any](m map[K]V) fmt.Stringer {
	return keys[K, V](m)
}

type keys[K comparable, V any] map[K]V

func (k keys[K, V]) String() string {
	return fmt.Sprintf(strings.Join(collections.Repeat("%v", len(k)), ", "), collections.Keys(k))
}

// Base64 logs lazily a byte array in base64 format
func Base64(b []byte) base64Enc {
	return b
}

type base64Enc []byte

func (b base64Enc) String() string {
	return base64.StdEncoding.EncodeToString(b)
}
