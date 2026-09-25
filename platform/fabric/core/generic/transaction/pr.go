/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ProposalResponse wraps a peer.ProposalResponse received from a
// counterparty, before its signature has been verified. Endorsement and
// Response are optional protobuf fields that a syntactically valid message
// can omit, so every accessor below reads them through the generated
// Get*() methods instead of dereferencing the pointers directly.
type ProposalResponse struct {
	pr      *peer.ProposalResponse
	results []byte
}

func NewProposalResponseFromResponse(proposalResponse *peer.ProposalResponse) (*ProposalResponse, error) {
	pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(proposalResponse.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "GetProposalResponsePayload error %s", err)
	}
	if len(pRespPayload.Extension) == 0 {
		return nil, errors.Errorf("empty pRespPayload.Extension")
	}
	respPayload, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
	if err != nil {
		return nil, errors.Wrapf(err, "failed GetChaincodeAction")
	}
	if len(respPayload.Results) == 0 {
		return nil, errors.Errorf("empty results")
	}

	return &ProposalResponse{
		pr:      proposalResponse,
		results: respPayload.Results,
	}, nil
}

func NewProposalResponseFromBytes(raw []byte) (*ProposalResponse, error) {
	proposalResponse := &peer.ProposalResponse{}
	if err := proto.Unmarshal(raw, proposalResponse); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
	}
	return NewProposalResponseFromResponse(proposalResponse)
}

func (p *ProposalResponse) Endorser() []byte {
	return p.pr.GetEndorsement().GetEndorser()
}

func (p *ProposalResponse) Payload() []byte {
	return p.pr.Payload
}

func (p *ProposalResponse) EndorserSignature() []byte {
	return p.pr.GetEndorsement().GetSignature()
}

func (p *ProposalResponse) Results() []byte {
	return p.results
}

func (p *ProposalResponse) PR() *peer.ProposalResponse {
	return p.pr
}

func (p *ProposalResponse) ResponseStatus() int32 {
	return p.pr.GetResponse().GetStatus()
}

func (p *ProposalResponse) ResponseMessage() string {
	return p.pr.GetResponse().GetMessage()
}

func (p *ProposalResponse) Bytes() ([]byte, error) {
	raw, err := proto.Marshal(p.pr)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (p *ProposalResponse) VerifyEndorsement(provider driver.VerifierProvider) error {
	endorser := view.Identity(p.pr.GetEndorsement().GetEndorser())
	v, err := provider.GetVerifier(endorser)
	if err != nil {
		return errors.Wrapf(err, "failed getting verifier for [%s]", endorser)
	}
	return v.Verify(append(p.Payload(), endorser...), p.EndorserSignature())
}
