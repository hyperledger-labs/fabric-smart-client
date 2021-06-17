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
	sp view2.ServiceProvider
}

func NewSigService(sp view2.ServiceProvider) *SigService {
	return &SigService{sp: sp}
}

func (s *SigService) GetVerifier(id view.Identity) (driver.Verifier, error) {
	return view2.GetSigService(s.sp).GetVerifier(id)
}

func (s *SigService) GetSigner(id view.Identity) (driver.Signer, error) {
	return view2.GetSigService(s.sp).GetSigner(id)
}

func (s *SigService) GetSigningIdentity(id view.Identity) (driver.SigningIdentity, error) {
	return view2.GetSigService(s.sp).GetSigningIdentity(id)
}

func (s *SigService) RegisterSigner(identity view.Identity, signer driver.Signer, verifier driver.Verifier) error {
	return view2.GetSigService(s.sp).RegisterSigner(identity, signer, verifier)
}
