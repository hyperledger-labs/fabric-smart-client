/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SigService struct {
	sigService *view2.SigService
}

func NewSigService(sp *view2.SigService) *SigService {
	return &SigService{sigService: sp}
}

func (s *SigService) GetVerifier(id view.Identity) (driver.Verifier, error) {
	return s.sigService.GetVerifier(id)
}

func (s *SigService) GetSigner(id view.Identity) (driver.Signer, error) {
	return s.sigService.GetSigner(id)
}

func (s *SigService) GetSigningIdentity(id view.Identity) (driver.SigningIdentity, error) {
	return s.sigService.GetSigningIdentity(id)
}

func (s *SigService) RegisterSigner(identity view.Identity, signer driver.Signer, verifier driver.Verifier) error {
	return s.sigService.RegisterSigner(identity, signer, verifier)
}
