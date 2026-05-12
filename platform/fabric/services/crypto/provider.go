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

type provider struct{}

func NewProvider() *provider {
	return &provider{}
}

// getDefaultFactory is a package-level variable that returns the default
// factory. It is assigned to factory.GetDefault by default and can be
// replaced in tests to simulate error conditions without changing
// production APIs.
// bccspFactory is a local interface with the small subset of methods
// used by this package. It is satisfied by the real `bccsp.BCCSP`.
type bccspFactory interface {
	Hash([]byte, bccsp.HashOpts) ([]byte, error)
	GetHash(bccsp.HashOpts) (hash.Hash, error)
}

// getDefaultFactory returns a bccspFactory. By default it wraps
// `factory.GetDefault`, but tests can replace it with a function
// returning a test double that implements `bccspFactory`.
var getDefaultFactory = func() bccspFactory { return factory.GetDefault() }

func (p *provider) Hash(msg []byte) ([]byte, error) {
	hash, err := getDefaultFactory().Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed computing SHA256 on [% x]", msg))
	}
	return hash, nil
}

func (p *provider) GetHash() hash.Hash {
	hash, err := getDefaultFactory().GetHash(&bccsp.SHA256Opts{})
	if err != nil {
		panic(errors.Errorf("failed getting SHA256"))
	}
	return hash
}
