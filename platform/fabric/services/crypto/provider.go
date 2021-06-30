/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package crypto

import (
	"fmt"
	"hash"

	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
)

type provider struct {
}

func NewProvider() *provider {
	return &provider{}
}

func (p *provider) Hash(msg []byte) ([]byte, error) {
	hash, err := factory.GetDefault().Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		panic(fmt.Errorf("failed computing SHA256 on [% x]", msg))
	}
	return hash, nil
}

func (p *provider) GetHash() hash.Hash {
	hash, err := factory.GetDefault().GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		panic(fmt.Errorf("failed getting SHA256"))
	}
	return hash
}
