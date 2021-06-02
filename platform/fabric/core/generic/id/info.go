/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package id

import (
	idemix2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/idemix"
	x5092 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func Info(id view.Identity, auditInfo []byte) string {
	d := &x5092.Deserializer{}
	info, err := d.Info(id, auditInfo)
	if err != nil {
		d2, err := idemix2.NewDeserializer(nil)
		if err != nil {
			return ""
		}
		info, err := d2.Info(id, auditInfo)
		if err != nil {
			return ""
		}
		return info
	}
	return info
}
