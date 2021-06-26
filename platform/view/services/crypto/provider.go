/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"crypto/sha256"
	"hash"

	"github.com/pkg/errors"
)

type provider struct {
}

func NewProvider() *provider {
	return &provider{}
}

func (p *provider) Hash(msg []byte) ([]byte, error) {
	hash := sha256.New()
	n, err := hash.Write(msg)
	if n != len(msg) {
		return nil, errors.Errorf("hash failure")
	}
	if err != nil {
		return nil, err
	}
	digest := hash.Sum(nil)

	return digest, nil
}

func (p *provider) GetHash() hash.Hash {
	return sha256.New()
}
