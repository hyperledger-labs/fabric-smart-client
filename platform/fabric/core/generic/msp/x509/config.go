/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package x509

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509/pkcs11"
)

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
