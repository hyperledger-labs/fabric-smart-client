/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-committer/utils/signature"
)

type VerifierProvider = driver.VerifierProvider

type ProposalResponse struct {
	pr *pb.ProposalResponse
}

func NewProposalResponseFromResponse(proposalResponse *pb.ProposalResponse) (*ProposalResponse, error) {
	return &ProposalResponse{
		pr: proposalResponse,
	}, nil
}

func NewProposalResponseFromBytes(raw []byte) (*ProposalResponse, error) {
	proposalResponse := &pb.ProposalResponse{}
	if err := proto.Unmarshal(raw, proposalResponse); err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling received proposal response")
	}
	return NewProposalResponseFromResponse(proposalResponse)
}

func (p *ProposalResponse) Endorser() []byte {
	return p.pr.Endorsement.Endorser
}

func (p *ProposalResponse) Payload() []byte {
	return p.pr.Payload
}

func (p *ProposalResponse) EndorserSignature() []byte {
	return p.pr.Endorsement.Signature
}

func (p *ProposalResponse) Results() []byte {
	return p.pr.GetPayload()
}

func (p *ProposalResponse) PR() *pb.ProposalResponse {
	return p.pr
}

func (p *ProposalResponse) ResponseStatus() int32 {
	return p.pr.Response.Status
}

func (p *ProposalResponse) ResponseMessage() string {
	return p.pr.Response.Message
}

func (p *ProposalResponse) Bytes() ([]byte, error) {
	raw, err := proto.Marshal(p.pr)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (p *ProposalResponse) VerifyEndorsement(provider VerifierProvider) error {
	// we first get the verifier for the endorser
	endorser := view.Identity(p.pr.Endorsement.Endorser)
	v, err := provider.GetVerifier(endorser)
	if err != nil {
		return errors.Wrapf(err, "failed getting verifier for [%s]", endorser)
	}

	// unmarshal payload to Tx
	var tx protoblocktx.Tx
	if err := proto.Unmarshal(p.pr.Payload, &tx); err != nil {
		return errors.Wrapf(err, "failed unmarshalling payload for [%s]", endorser)
	}

	var sigs [][]byte
	if err := json.Unmarshal(p.EndorserSignature(), &sigs); err != nil {
		return err
	}

	// check that we have a signature for each namespace
	if len(tx.GetNamespaces()) != len(sigs) {
		return fmt.Errorf("mismatch number of signatures and namespaces")
	}

	// TODO get the txID from a better place
	// get the txID
	txID := p.PR().GetResponse().GetMessage()

	// check each namespace signature with the corresponding signature using the endorser verifier
	for idx, ns := range tx.GetNamespaces() {
		digest, err := signature.ASN1MarshalTxNamespace(txID, ns)
		if err != nil {
			return fmt.Errorf("cannot serialize tx: %w", err)
		}

		if err := v.Verify(digest, sigs[idx]); err != nil {
			return err
		}
	}

	return nil
}
