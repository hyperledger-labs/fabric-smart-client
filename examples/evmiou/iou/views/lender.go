/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// CreateIOUResponderView is the lender's responder to CreateIOUView. It trusts the session for
// the borrower's identity, fills in its own address as the lender, checks the create rules and
// returns its EIP-712 and FSC signatures over the Create digest.
type CreateIOUResponderView struct{}

// Call runs the lender's side of a create.
func (*CreateIOUResponderView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	s := session.JSON(viewCtx)
	var p CreateProposal
	if err := s.ReceiveWithTimeout(&p, receiveTimeout); err != nil {
		return nil, errors.Wrap(err, "lender: failed receiving the proposal")
	}

	c := contract.Create{ID: p.ID, Amount: p.Amount, Borrower: p.Borrower, Lender: svc.Signer.Address()}
	existing, err := getIOU(viewCtx, svc, c.ID)
	if err != nil {
		return nil, err
	}
	if err := ValidateCreate(c, existing); err != nil {
		return nil, errors.Wrap(err, "lender refused")
	}

	digest := eip712.Digest(svc.Domain(), c.HashStruct())
	evmSig, err := svc.Signer.Sign(digest)
	if err != nil {
		return nil, errors.Wrap(err, "lender: failed signing")
	}
	fscSig, err := signFSC(viewCtx, s.Info(), digest)
	if err != nil {
		return nil, errors.Wrap(err, "lender: failed FSC signing")
	}

	return nil, s.Send(viewCtx.Context(), &CreateEndorsement{Create: c, EVMSig: evmSig, FSCSig: fscSig})
}

// UpdateIOUResponderView is the lender's responder to UpdateIOUView. It reads the stored IOU,
// checks it is the stored lender and the update rules, and returns its EIP-712 signature.
type UpdateIOUResponderView struct{}

// Call runs the lender's side of an update.
func (*UpdateIOUResponderView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	s := session.JSON(viewCtx)
	var p UpdateProposal
	if err := s.ReceiveWithTimeout(&p, receiveTimeout); err != nil {
		return nil, errors.Wrap(err, "lender: failed receiving the proposal")
	}

	stored, err := getIOU(viewCtx, svc, p.Update.ID)
	if err != nil {
		return nil, err
	}
	if err := ValidateUpdate(stored, p.Update); err != nil {
		return nil, errors.Wrap(err, "lender refused")
	}
	if me := svc.Signer.Address(); stored.Lender != me {
		return nil, errors.Errorf("lender refused: the stored lender is [%s], not [%s]", stored.Lender, me)
	}

	evmSig, err := svc.Signer.Sign(eip712.Digest(svc.Domain(), p.Update.HashStruct()))
	if err != nil {
		return nil, errors.Wrap(err, "lender: failed signing")
	}

	return nil, s.Send(viewCtx.Context(), &UpdateEndorsement{EVMSig: evmSig})
}
