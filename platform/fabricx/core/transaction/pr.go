/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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
	if p.pr.Endorsement == nil {
		return nil
	}
	return p.pr.Endorsement.Endorser
}

func (p *ProposalResponse) Payload() []byte {
	return p.pr.Payload
}

func (p *ProposalResponse) EndorserSignature() []byte {
	if p.pr.Endorsement == nil {
		return nil
	}
	return p.pr.Endorsement.Signature
}

func (p *ProposalResponse) Results() []byte {
	return p.pr.GetPayload()
}

func (p *ProposalResponse) PR() *pb.ProposalResponse {
	return p.pr
}

func (p *ProposalResponse) ResponseStatus() int32 {
	if p.pr.Response == nil {
		return 0
	}
	return p.pr.Response.Status
}

func (p *ProposalResponse) ResponseMessage() string {
	if p.pr.Response == nil {
		return ""
	}
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
	endorsements, err := unmarshalEndorsementsFromProposalResponse(p.EndorserSignature())
	if err != nil {
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
			return errors.Wrapf(err, "failed asn1 marshal for [txID=%s] [ns=%s]", txID, ns)
		}

		items := endorsements[idx].GetEndorsementsWithIdentity()
		if len(items) == 0 {
			return errors.Errorf("missing endorsement for [txID=%s] [ns=%s]", txID, ns)
		}

		// note that we are checking the endorsement returned via a proposal response from the endorser
		// that is, at this stage it should only contain "a single" signature (endorsement) per namespace;
		sig := items[0].GetEndorsement()

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

func unmarshalEndorsementsFromProposalResponse(raw []byte) ([]*applicationpb.Endorsements, error) {
	var rawEndorsements [][]byte
	if err := json.Unmarshal(raw, &rawEndorsements); err != nil {
		return nil, errors.Wrap(err, "unmarshal serialized endorsements")
	}

	endorsements := make([]*applicationpb.Endorsements, len(rawEndorsements))
	for i, item := range rawEndorsements {
		if len(item) == 0 {
			endorsements[i] = &applicationpb.Endorsements{}
			continue
		}

		e := &applicationpb.Endorsements{}
		if err := proto.Unmarshal(item, e); err != nil {
			return nil, errors.Wrapf(err, "unmarshal endorsement at index %d", i)
		}
		endorsements[i] = e
	}

	return endorsements, nil
}

func marshalEndorsementsForProposalResponse(endorsements []*applicationpb.Endorsements) ([]byte, error) {
	rawEndorsements := make([][]byte, len(endorsements))
	for i, endorsement := range endorsements {
		if endorsement == nil {
			rawEndorsements[i] = nil
			continue
		}

		raw, err := proto.Marshal(endorsement)
		if err != nil {
			return nil, errors.Wrapf(err, "marshal endorsement at index %d", i)
		}
		rawEndorsements[i] = raw
	}

	raw, err := json.Marshal(rawEndorsements)
	if err != nil {
		return nil, errors.Wrap(err, "marshal serialized endorsements")
	}
	return raw, nil
}
