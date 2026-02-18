/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
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
		return nil, errors.Wrap(err, "unmarshal proposal response")
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
		return nil, errors.Wrap(err, "marshal proposal response")
	}
	return raw, nil
}

func (p *ProposalResponse) VerifyEndorsement(provider VerifierProvider) error {
	// we first get the verifier for the endorser
	endorser := view.Identity(p.pr.Endorsement.Endorser)
	v, err := provider.GetVerifier(endorser)
	if err != nil {
		return errors.Wrapf(err, "getting verifier for [%s]", endorser)
	}

	// unmarshal payload to Tx
	var tx applicationpb.Tx
	if err := proto.Unmarshal(p.pr.Payload, &tx); err != nil {
		return errors.Wrapf(err, "unmarshal proposal response payload for [%s]", endorser)
	}

	// unmarshal endorsement signatures for each namespace
	var endorsements []*applicationpb.Endorsements
	if err := json.Unmarshal(p.EndorserSignature(), &endorsements); err != nil {
		return errors.Wrap(err, "unmarshal endorsement signatures")
	}

	// check that we have a signature for each namespace
	if len(tx.GetNamespaces()) != len(endorsements) {
		return errors.New("mismatch number of signatures and namespaces")
	}

	// get the txID from metadata
	txID := string(p.PR().GetResponse().GetPayload())

	// check each namespace signature with the corresponding signature using the endorser verifier
	for idx, ns := range tx.GetNamespaces() {

		digest, err := tx.Namespaces[idx].ASN1Marshal(txID)
		if err != nil {
			return errors.Wrapf(err, "failed asn1 marshalfor [txID=%s] [ns=%s]", txID, ns)
		}

		// note that we are checking the endorsement returned via a proposal response from the endorser
		// that is, at this stage it should only contain "a single" signature (endorsement) per namespace;
		sig := endorsements[idx].GetEndorsementsWithIdentity()[0].GetEndorsement()

		// TODO: check the type of the endorsement
		// If msp-based with or without attached identity - we need to check it corresponds to the endorser identity
		// as we specify above using `view.Identity(p.pr.Endorsement.Endorser)`.

		// TODO: for threshold-based endorsement we need to first aggregate signatures shares before verifying.

		if err := v.Verify(digest, sig); err != nil {
			return errors.Wrapf(err, "invalid namespace signature for [txID=%s] [ns=%s]", txID, ns)
		}
	}

	return nil
}
