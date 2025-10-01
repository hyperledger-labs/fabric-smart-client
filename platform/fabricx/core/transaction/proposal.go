/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

type Proposal struct {
	p *pb.Proposal
}

func (p *Proposal) Header() []byte {
	return p.p.Header
}

func (p *Proposal) Payload() []byte {
	return p.p.Payload
}

type SignedProposal struct {
	s  *pb.SignedProposal
	up *transaction.UnpackedProposal
}

func newSignedProposal(s *pb.SignedProposal) (*SignedProposal, error) {
	up, err := transaction.UnpackSignedProposal(s)
	if err != nil {
		return nil, err
	}
	logger.Debugf("new signed proposal with proposal hash = %x", up.ProposalHash)
	return &SignedProposal{s: s, up: up}, nil
}

func (p *SignedProposal) ProposalBytes() []byte {
	return p.s.ProposalBytes
}

func (p *SignedProposal) Signature() []byte {
	return p.s.Signature
}

func (p *SignedProposal) ProposalHash() []byte {
	return p.up.ProposalHash
}

func (p *SignedProposal) ChaincodeName() string {
	return p.up.ChaincodeName
}

func (p *SignedProposal) ChaincodeVersion() string {
	return p.up.ChaincodeVersion
}
