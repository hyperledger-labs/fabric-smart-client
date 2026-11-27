/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// CreateProposal is what the borrower sends the lender to start a create: the lender fills in
// its own address and signs.
type CreateProposal struct {
	ID       [32]byte
	Amount   uint64
	Borrower client.Address
}

// CreateEndorsement is the lender's reply to a CreateProposal: the full Create struct and the
// lender's EIP-712 and FSC signatures over its EIP-712 digest.
type CreateEndorsement struct {
	Create         contract.Create
	EVMSig, FSCSig []byte
}

// UpdateProposal is what the borrower sends the lender to start an update.
type UpdateProposal struct {
	Update contract.Update
}

// UpdateEndorsement is the lender's reply to an UpdateProposal: its EIP-712 signature over the
// Update digest.
type UpdateEndorsement struct {
	EVMSig []byte
}

// ApprovalRequest is what the borrower sends an approver. Exactly one of Create and Update is
// set. For a create it also carries the lender's FSC identity and both parties' FSC signatures;
// for an update only the EIP-712 signatures are set.
type ApprovalRequest struct {
	Create *contract.Create
	Update *contract.Update

	LenderID view.Identity

	BorrowerEVMSig, BorrowerFSCSig []byte
	LenderEVMSig, LenderFSCSig     []byte
}

// ApprovalResponse is the approver's reply: its EIP-712 signature.
type ApprovalResponse struct {
	EVMSig []byte
}
