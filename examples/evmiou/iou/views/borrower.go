/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"crypto/rand"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/chain"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/ecdsa"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// CreateIOUView is the borrower's view that creates an IOU of Amount owed to the FSC node named
// Lender, approved by the FSC node named Approver. It returns the IOU id as 0x-prefixed hex.
type CreateIOUView struct {
	Amount           uint64
	Lender, Approver string

	// ForgeLenderID is for tests only: the borrower forwards the real lender identity but replaces
	// the lender FSC signature with one by a freshly generated key. The approver must refuse it.
	ForgeLenderID bool
}

// Call collects the lender's and the approver's signatures, submits create and waits for the
// receipt.
func (v *CreateIOUView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	ep := endpoint.GetService(viewCtx)
	lenderID, err := ep.GetIdentity(v.Lender, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed resolving lender [%s]", v.Lender)
	}
	approverID, err := ep.GetIdentity(v.Approver, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed resolving approver [%s]", v.Approver)
	}

	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, errors.Wrap(err, "failed generating the iou id")
	}
	p := CreateProposal{ID: crypto.Keccak256Hash(seed[:]), Amount: v.Amount, Borrower: svc.Signer.Address()}

	// The lender fills in its address and signs the Create struct both ways.
	ls, err := session.NewJSON(viewCtx, viewCtx.Initiator(), lenderID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening a session to lender [%s]", v.Lender)
	}
	if err := ls.Send(viewCtx.Context(), &p); err != nil {
		return nil, errors.Wrapf(err, "failed sending the proposal to lender [%s]", v.Lender)
	}
	var e CreateEndorsement
	if err := ls.ReceiveWithTimeout(&e, receiveTimeout); err != nil {
		return nil, errors.Wrapf(err, "lender [%s] did not endorse the create", v.Lender)
	}
	c := e.Create
	if c.ID != p.ID || c.Amount != p.Amount || c.Borrower != p.Borrower {
		return nil, errors.Errorf("lender [%s] endorsed a different create", v.Lender)
	}
	digest := eip712.Digest(svc.Domain(), c.HashStruct())
	if err := verifyEVM(digest, e.EVMSig, c.Lender, "lender"); err != nil {
		return nil, err
	}
	if err := verifyFSC(viewCtx, lenderID, digest, e.FSCSig, "lender"); err != nil {
		return nil, err
	}

	// The borrower signs too and asks the approver.
	as, err := session.NewJSON(viewCtx, viewCtx.Initiator(), approverID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening a session to approver [%s]", v.Approver)
	}
	borrowerEVMSig, err := svc.Signer.Sign(digest)
	if err != nil {
		return nil, errors.Wrap(err, "borrower: failed signing")
	}
	borrowerFSCSig, err := signFSC(viewCtx, as.Info(), digest)
	if err != nil {
		return nil, errors.Wrap(err, "borrower: failed FSC signing")
	}
	lenderFSCSig := e.FSCSig
	if v.ForgeLenderID {
		if lenderFSCSig, err = forgeSignature(digest); err != nil {
			return nil, err
		}
	}
	if err := as.Send(viewCtx.Context(), &ApprovalRequest{
		Create:         &c,
		LenderID:       lenderID,
		BorrowerEVMSig: borrowerEVMSig,
		BorrowerFSCSig: borrowerFSCSig,
		LenderEVMSig:   e.EVMSig,
		LenderFSCSig:   lenderFSCSig,
	}); err != nil {
		return nil, errors.Wrapf(err, "failed sending the request to approver [%s]", v.Approver)
	}
	var resp ApprovalResponse
	if err := as.ReceiveWithTimeout(&resp, receiveTimeout); err != nil {
		return nil, errors.Wrapf(err, "approver [%s] did not approve the create", v.Approver)
	}

	if _, err := submit(viewCtx.Context(), svc, contract.EncodeCreate(c, borrowerEVMSig, e.EVMSig, resp.EVMSig)); err != nil {
		return nil, err
	}

	return client.Hash(c.ID).Hex(), nil
}

// CreateIOUViewFactory builds a CreateIOUView from its JSON input.
type CreateIOUViewFactory struct{}

// NewView unmarshals in into a CreateIOUView.
func (*CreateIOUViewFactory) NewView(in []byte) (view.View, error) {
	v := &CreateIOUView{}
	if err := json.Unmarshal(in, v); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling the create input")
	}

	return v, nil
}

// UpdateIOUView is the borrower's view that lowers the IOU with ID, a 0x-prefixed hex id, to
// Amount, endorsed by the FSC node named Lender and approved by the one named Approver. It
// returns the transaction hash as 0x-prefixed hex.
type UpdateIOUView struct {
	ID               string
	Amount           uint64
	Lender, Approver string
}

// Call reads the stored IOU, collects the lender's and the approver's signatures over the
// update, submits it and waits for the receipt.
func (v *UpdateIOUView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	id, err := client.HexToHash(v.ID)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid iou id [%s]", v.ID)
	}
	ep := endpoint.GetService(viewCtx)
	lenderID, err := ep.GetIdentity(v.Lender, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed resolving lender [%s]", v.Lender)
	}
	approverID, err := ep.GetIdentity(v.Approver, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed resolving approver [%s]", v.Approver)
	}

	// The rules are left to the lender and the approver: the borrower only checks that the
	// IOU exists and is its own.
	stored, err := getIOU(viewCtx, svc, id)
	if err != nil {
		return nil, err
	}
	if stored.Version == 0 {
		return nil, errors.Errorf("iou [%s] does not exist", v.ID)
	}
	if me := svc.Signer.Address(); stored.Borrower != me {
		return nil, errors.Errorf("the stored borrower is [%s], not [%s]", stored.Borrower, me)
	}
	u := contract.Update{ID: id, Amount: v.Amount, Version: stored.Version}
	digest := eip712.Digest(svc.Domain(), u.HashStruct())

	ls, err := session.NewJSON(viewCtx, viewCtx.Initiator(), lenderID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening a session to lender [%s]", v.Lender)
	}
	if err := ls.Send(viewCtx.Context(), &UpdateProposal{Update: u}); err != nil {
		return nil, errors.Wrapf(err, "failed sending the proposal to lender [%s]", v.Lender)
	}
	var e UpdateEndorsement
	if err := ls.ReceiveWithTimeout(&e, receiveTimeout); err != nil {
		return nil, errors.Wrapf(err, "lender [%s] did not endorse the update", v.Lender)
	}
	if err := verifyEVM(digest, e.EVMSig, stored.Lender, "lender"); err != nil {
		return nil, err
	}

	borrowerSig, err := svc.Signer.Sign(digest)
	if err != nil {
		return nil, errors.Wrap(err, "borrower: failed signing")
	}
	as, err := session.NewJSON(viewCtx, viewCtx.Initiator(), approverID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed opening a session to approver [%s]", v.Approver)
	}
	if err := as.Send(viewCtx.Context(), &ApprovalRequest{
		Update:         &u,
		BorrowerEVMSig: borrowerSig,
		LenderEVMSig:   e.EVMSig,
	}); err != nil {
		return nil, errors.Wrapf(err, "failed sending the request to approver [%s]", v.Approver)
	}
	var resp ApprovalResponse
	if err := as.ReceiveWithTimeout(&resp, receiveTimeout); err != nil {
		return nil, errors.Wrapf(err, "approver [%s] did not approve the update", v.Approver)
	}

	h, err := submit(viewCtx.Context(), svc, contract.EncodeUpdate(u, borrowerSig, e.EVMSig, resp.EVMSig))
	if err != nil {
		return nil, err
	}

	return h.Hex(), nil
}

// UpdateIOUViewFactory builds an UpdateIOUView from its JSON input.
type UpdateIOUViewFactory struct{}

// NewView unmarshals in into an UpdateIOUView.
func (*UpdateIOUViewFactory) NewView(in []byte) (view.View, error) {
	v := &UpdateIOUView{}
	if err := json.Unmarshal(in, v); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling the update input")
	}

	return v, nil
}

// forgeSignature signs digest with a freshly generated ECDSA key, unrelated to the lender's real
// FSC identity. It backs ForgeLenderID.
func forgeSignature(digest [32]byte) ([]byte, error) {
	_, signer, _, err := ecdsa.NewSigner()
	if err != nil {
		return nil, errors.Wrap(err, "failed generating a forged key")
	}
	sig, err := signer.Sign(digest[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed signing with the forged key")
	}

	return sig, nil
}

// submit sends data to the IOU contract and waits for a successful receipt. A call that would
// revert usually fails earlier, at Send's gas estimation, since the node refuses to estimate one;
// chain.ErrReverted is only reached when the transaction is broadcast anyway (as with a fixed gas
// limit) and reverts on execution.
func submit(ctx context.Context, svc *iou.Service, data []byte) (client.Hash, error) {
	h, err := svc.Submitter.Send(ctx, &svc.Contract, nil, data)
	if err != nil {
		return client.Hash{}, errors.Wrap(err, "failed submitting the transaction")
	}
	if _, err := chain.WaitReceipt(ctx, svc.Client, h, svc.PollInterval, svc.Timeout); err != nil {
		return client.Hash{}, err
	}

	return h, nil
}
