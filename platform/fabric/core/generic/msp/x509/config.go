/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"github.com/go-viper/mapstructure/v2"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509/pkcs11"
)

func ToBCCSPOpts(boxed interface{}) (*config.BCCSP, error) {
	opts := &config.BCCSP{}
	cfg := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true, // allow pin to be a string
		Result:           &opts,
	}

	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return nil, err
	}

	err = decoder.Decode(boxed)
	if err != nil {
		return nil, err
	}
	return opts, nil
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
