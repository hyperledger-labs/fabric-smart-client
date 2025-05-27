/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"hash"

	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	"github.com/pkg/errors"
)

type provider struct {
}

func NewProvider() *provider {
	return &provider{}
}

func (p *provider) Hash(msg []byte) ([]byte, error) {
	hash, err := factory.GetDefault().Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed computing SHA256 on [% x]", msg))
	}
	return hash, nil
}

func (p *provider) GetHash() hash.Hash {
	hash, err := factory.GetDefault().GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed getting SHA256"))
	}
	return hash
}
