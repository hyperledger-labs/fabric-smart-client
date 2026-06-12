/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"hash"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type hasher interface {
	Hash(msg []byte, opts bccsp.HashOpts) ([]byte, error)
	GetHash(opts bccsp.HashOpts) (hash.Hash, error)
}

type provider struct {
	hasher hasher
}

func NewProvider() *provider {
	return &provider{hasher: factory.GetDefault()}
}

func (p *provider) Hash(msg []byte) ([]byte, error) {
	hash, err := p.hasher.Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed computing SHA256 on [% x]", msg))
	}
	return hash, nil
}

func (p *provider) GetHash() hash.Hash {
	hash, err := p.hasher.GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed getting SHA256"))
	}
	return hash
}
