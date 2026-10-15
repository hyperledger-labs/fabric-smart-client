//go:build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/pkcs11"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

type (
	PKCS11Opts   = pkcs11.PKCS11Opts
	KeyIDMapping = pkcs11.KeyIDMapping
)

// NewProvider returns a PKCS#11-backed BCCSP that uses mapper to resolve
// key SKIs to HSM key identifiers.
func NewProvider(opts pkcs11.PKCS11Opts, ks bccsp.KeyStore, mapper func(ski []byte) []byte) (*pkcs11.Provider, error) {
	csp, err := pkcs11.New(opts, ks, pkcs11.WithKeyMapper(mapper))
	if err != nil {
		return nil, errors.WithMessagef(err, "Failed initializing PKCS11 library with config [%+v]", opts)
	}
	return csp, nil
}
