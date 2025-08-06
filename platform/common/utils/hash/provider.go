/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"crypto/sha256"
	"hash"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type SHA256Provider struct {
}

func NewSHA256Provider() *SHA256Provider {
	return &SHA256Provider{}
}

func (p *SHA256Provider) Hash(msg []byte) ([]byte, error) {
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

func (p *SHA256Provider) GetHash() hash.Hash {
	return sha256.New()
}
