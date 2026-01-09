/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricutils

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/kvledger/rwsetutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

// UnpackedProposalResponse contains the interesting artifacts from inside the proposal.
type UnpackedProposalResponse struct {
	ChaincodeAction *pb.ChaincodeAction
	TxRwSet         *rwsetutil.TxRwSet
}

func (p *UnpackedProposalResponse) Results() []byte {
	return p.ChaincodeAction.Results
}

// UnpackProposalResponse creates an an *UnpackedProposalResponse which is guaranteed to have
// no zero-ed fields or it returns an error.
func UnpackProposalResponse(payload []byte) (*UnpackedProposalResponse, error) {
	prop, err := protoutil.UnmarshalProposalResponsePayload(payload)
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
		ChaincodeAction: chAction,
		TxRwSet:         txRWSet,
	}, nil
}
