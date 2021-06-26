/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"crypto/sha256"

	"github.com/pkg/errors"
)

func SHA256(raw []byte) ([]byte, error) {
	hash := sha256.New()
	n, err := hash.Write(raw)
	if n != len(raw) {
		return nil, errors.Errorf("hash failure")
	}
	if err != nil {
		return nil, err
	}
	digest := hash.Sum(nil)

	return digest, nil
}

func SHA256OrPanic(raw []byte) []byte {
	hash := sha256.New()
	n, err := hash.Write(raw)
	if n != len(raw) {
		panic("hash failure")
	}
	if err != nil {
		panic(err)
	}
	digest := hash.Sum(nil)

	return digest
}
