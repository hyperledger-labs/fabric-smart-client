/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/core/ledger/kvledger/txmgmt/rwsetutil"
	"github.com/hyperledger/fabric/protoutil"
)

// UnpackedProposalResponse contains the interesting artifacts from inside the proposal.
type UnpackedProposalResponse struct {
	ProposalResponse *pb.ProposalResponse
	ChaincodeAction  *pb.ChaincodeAction
	TxRwSet          *rwsetutil.TxRwSet
}

func (p *UnpackedProposalResponse) Results() []byte {
	return p.ChaincodeAction.Results
}

// UnpackProposalResponse creates an an *UnpackedProposalResponse which is guaranteed to have
// no zero-ed fields or it returns an error.
func UnpackProposalResponse(proposalResponse *pb.ProposalResponse) (*UnpackedProposalResponse, error) {
	prop, err := protoutil.UnmarshalProposalResponsePayload(proposalResponse.Payload)
	if err != nil {
		return nil, err
	}

	chAction, err := protoutil.UnmarshalChaincodeAction(prop.Extension)
	if err != nil {
		return nil, err
	}

	txRWSet := &rwsetutil.TxRwSet{}
	if err = txRWSet.FromProtoBytes(chAction.Results); err != nil {
		return nil, err
	}

	return &UnpackedProposalResponse{
		ProposalResponse: proposalResponse,
		ChaincodeAction:  chAction,
		TxRwSet:          txRWSet,
	}, nil
}
