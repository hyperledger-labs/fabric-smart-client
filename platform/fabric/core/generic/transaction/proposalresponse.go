/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

// UnpackedProposalResponse contains the interesting artifacts from inside the proposal.
type UnpackedProposalResponse struct {
	ProposalResponse *pb.ProposalResponse
	ChaincodeAction  *pb.ChaincodeAction
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

	return &UnpackedProposalResponse{
		ProposalResponse: proposalResponse,
		ChaincodeAction:  chAction,
	}, nil
}
