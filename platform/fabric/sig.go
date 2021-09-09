/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SignerService struct {
	sigService driver.SignerService
}

func (s *SignerService) GetSigner(id view.Identity) (Signer, error) {
	return s.sigService.GetSigner(id)
}
