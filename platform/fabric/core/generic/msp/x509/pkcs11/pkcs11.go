//go:build pkcs11

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pkcs11

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger/fabric-lib-go/bccsp"
	"github.com/hyperledger/fabric-lib-go/bccsp/pkcs11"
)

type (
	PKCS11Opts   = pkcs11.PKCS11Opts
	KeyIDMapping = pkcs11.KeyIDMapping
)

func NewProvider(opts pkcs11.PKCS11Opts, ks bccsp.KeyStore, mapper func(ski []byte) []byte) (*pkcs11.Provider, error) {
	csp, err := pkcs11.New(opts, ks, pkcs11.WithKeyMapper(mapper))
	if err != nil {
		return nil, errors.WithMessagef(err, "Failed initializing PKCS11 library with config [%+v]", opts)
	}
	return csp, nil
}

func ToPKCS11OptsOpts(o *config.PKCS11) *pkcs11.PKCS11Opts {
	res := &pkcs11.PKCS11Opts{
		Security:       o.Security,
		Hash:           o.Hash,
		Library:        o.Library,
		Label:          o.Label,
		Pin:            o.Pin,
		SoftwareVerify: o.SoftwareVerify,
		Immutable:      o.Immutable,
		AltID:          o.AltID,
	}
	for _, d := range o.KeyIDs {
		res.KeyIDs = append(res.KeyIDs, pkcs11.KeyIDMapping{
			SKI: d.SKI,
			ID:  d.ID,
		})
	}
	return res
}
