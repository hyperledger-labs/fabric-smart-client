/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/pkg/errors"
)

func HashUInt64(raw []byte) (uint64, error) {
	digest, err := SHA256(raw)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(digest[:8]), nil
}

func HashInt64(raw []byte) (int64, error) {
	digest, err := HashUInt64(raw)
	if err != nil {
		return 0, err
	}
	return int64(digest >> 1), nil
}

func HashUInt32(raw []byte) (uint32, error) {
	digest, err := SHA256(raw)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(digest[:4]), nil
}

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
