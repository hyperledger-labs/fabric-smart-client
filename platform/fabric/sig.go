/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package fabric

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SigService struct {
	sigService driver.SigService
}

func (s *SigService) GetVerifier(id view.Identity) (Verifier, error) {
	return s.sigService.GetVerifier(id)
}

func (s *SigService) GetSigner(id view.Identity) (Signer, error) {
	return s.sigService.GetSigner(id)
}
