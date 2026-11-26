/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// checkLenderID refuses a forwarded lender identity that is not a known FSC peer, or that is the
// borrower's. The FSC verifier accepts any well-formed public key, so the resolver map is the
// allow-list that stops a borrower from forwarding a key it generated itself.
func checkLenderID(ctx context.Context, ep *endpoint.Service, borrowerID, lenderID view.Identity) error {
	resolver, err := ep.GetResolver(ctx, lenderID)
	if err != nil {
		return errors.Wrap(err, "the lender's FSC identity is unknown")
	}
	if resolver.GetId().Equal(borrowerID) {
		return errors.New("the lender's FSC identity is the borrower's")
	}

	return nil
}

// verifyFSC checks that sig is id's FSC signature over digest. It uses LookupVerifier because id
// may be chosen by the remote party, and GetVerifier would cache it for good.
func verifyFSC(viewCtx view.Context, id view.Identity, digest [32]byte, signature []byte, who string) error {
	sigSvc, err := sig.GetService(viewCtx)
	if err != nil {
		return err
	}
	verifier, err := sigSvc.LookupVerifier(id)
	if err != nil {
		return errors.Wrapf(err, "failed getting the %s's FSC verifier", who)
	}

	return errors.Wrapf(verifier.Verify(digest[:], signature), "invalid %s FSC signature", who)
}

// signFSC signs digest with the local FSC identity of the session described by info.
func signFSC(viewCtx view.Context, info view.SessionInfo, digest [32]byte) ([]byte, error) {
	localID, err := endpoint.GetService(viewCtx).GetIdentity("", info.LocalPKID)
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving the local identity")
	}
	sigSvc, err := sig.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	signer, err := sigSvc.GetSigner(localID)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the local FSC signer")
	}

	return signer.Sign(digest[:])
}
