/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/eip712"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// receiveTimeout bounds every wait for the other party's message, as in the Fabric IOU.
const receiveTimeout = time.Minute

// ApproverView is the approver's responder to CreateIOUView and UpdateIOUView. It checks the
// request against the IOU rules and the on-chain state and replies with its EIP-712 signature;
// returning an error sends the refusal back to the borrower.
type ApproverView struct{}

// Call runs the approver's side of a create or an update.
func (*ApproverView) Call(viewCtx view.Context) (any, error) {
	svc, err := iou.GetService(viewCtx)
	if err != nil {
		return nil, err
	}
	s := session.JSON(viewCtx)
	var req ApprovalRequest
	if err := s.ReceiveWithTimeout(&req, receiveTimeout); err != nil {
		return nil, errors.Wrap(err, "approver: failed receiving the request")
	}

	var digest [32]byte
	switch {
	case req.Create != nil && req.Update == nil:
		digest, err = checkCreate(viewCtx, svc, s.Info(), &req)
	case req.Update != nil && req.Create == nil:
		digest, err = checkUpdate(viewCtx, svc, &req)
	default:
		err = errors.New("expected exactly one of create and update")
	}
	if err != nil {
		return nil, errors.Wrap(err, "approver refused")
	}

	approverSig, err := svc.Signer.Sign(digest)
	if err != nil {
		return nil, errors.Wrap(err, "approver: failed signing")
	}

	return nil, s.Send(viewCtx.Context(), &ApprovalResponse{EVMSig: approverSig})
}

// checkCreate checks a create request and returns the digest to sign. The borrower's FSC
// identity is the one authenticated on this session, the lender's is the forwarded LenderID;
// both FSC signatures bind those identities to the addresses in the Create struct.
func checkCreate(viewCtx view.Context, svc *iou.Service, info view.SessionInfo, req *ApprovalRequest) ([32]byte, error) {
	c := *req.Create
	existing, err := getIOU(viewCtx, svc, c.ID)
	if err != nil {
		return [32]byte{}, err
	}
	if err := ValidateCreate(c, existing); err != nil {
		return [32]byte{}, err
	}

	ep := endpoint.GetService(viewCtx)
	borrowerID, err := ep.GetIdentity("", info.RemotePKID)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "failed resolving the borrower's identity")
	}
	if err := checkLenderID(viewCtx.Context(), ep, borrowerID, req.LenderID); err != nil {
		return [32]byte{}, err
	}

	digest := eip712.Digest(svc.Domain(), c.HashStruct())
	if err := verifyFSC(viewCtx, borrowerID, digest, req.BorrowerFSCSig, "borrower"); err != nil {
		return [32]byte{}, err
	}
	if err := verifyFSC(viewCtx, req.LenderID, digest, req.LenderFSCSig, "lender"); err != nil {
		return [32]byte{}, err
	}
	if err := verifyEVM(digest, req.BorrowerEVMSig, c.Borrower, "borrower"); err != nil {
		return [32]byte{}, err
	}
	if err := verifyEVM(digest, req.LenderEVMSig, c.Lender, "lender"); err != nil {
		return [32]byte{}, err
	}

	return digest, nil
}

// checkUpdate checks an update request against the stored IOU and returns the digest to sign.
func checkUpdate(viewCtx view.Context, svc *iou.Service, req *ApprovalRequest) ([32]byte, error) {
	u := *req.Update
	stored, err := getIOU(viewCtx, svc, u.ID)
	if err != nil {
		return [32]byte{}, err
	}
	if err := ValidateUpdate(stored, u); err != nil {
		return [32]byte{}, err
	}

	digest := eip712.Digest(svc.Domain(), u.HashStruct())
	if err := verifyEVM(digest, req.BorrowerEVMSig, stored.Borrower, "borrower"); err != nil {
		return [32]byte{}, err
	}
	if err := verifyEVM(digest, req.LenderEVMSig, stored.Lender, "lender"); err != nil {
		return [32]byte{}, err
	}

	return digest, nil
}

// verifyEVM checks that sig is an EIP-712 signature over digest by want.
func verifyEVM(digest [32]byte, sig []byte, want client.Address, who string) error {
	got, err := eip712.RecoverAddress(digest, sig)
	if err != nil {
		return errors.Wrapf(err, "invalid %s EIP-712 signature", who)
	}
	if got != want {
		return errors.Errorf("%s EIP-712 signature is by [%s], expected [%s]", who, got, want)
	}

	return nil
}
