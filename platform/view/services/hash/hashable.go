/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"crypto/sha256"
	"encoding/base64"
)

type Hashable []byte

func (id Hashable) String() string {
	if len(id) == 0 {
		return ""
	}
	hash := sha256.New()
	n, err := hash.Write(id)
	if n != len(id) {
		panic("hash failure")
	}
	if err != nil {
		panic(err)
	}
	digest := hash.Sum(nil)
	return base64.StdEncoding.EncodeToString(digest)
}

func (id Hashable) RawString() string {
	if len(id) == 0 {
		return ""
	}
	hash := sha256.New()
	n, err := hash.Write(id)
	if n != len(id) {
		panic("hash failure")
	}
	if err != nil {
		panic(err)
	}
	digest := hash.Sum(nil)
	return string(digest)
}
