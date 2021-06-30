/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sig

import (
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TODO: remove this
func Sign(sp driver.ServiceProvider, id view.Identity, payload []byte) ([]byte, error) {
	signer, err := view2.GetSigService(sp).GetSigner(id)
	if err != nil {
		return nil, err
	}
	return signer.Sign(payload)
}

// TODO: remove this
func GetSigner(sp driver.ServiceProvider, id view.Identity) (driver.Signer, error) {
	signer, err := view2.GetSigService(sp).GetSigner(id)
	if err != nil {
		return nil, err
	}
	return signer, nil
}
