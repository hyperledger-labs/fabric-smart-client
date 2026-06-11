/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"

type SerializableSigner struct {
	Serialized   []byte
	SerializeErr error
	Signature    []byte
	SignErr      error
}

func (s *SerializableSigner) Sign(_ []byte) ([]byte, error) {
	if s.SignErr != nil {
		return nil, s.SignErr
	}
	if len(s.Signature) == 0 {
		return []byte("signature"), nil
	}
	return s.Signature, nil
}

func (s *SerializableSigner) Serialize() ([]byte, error) {
	if s.SerializeErr != nil {
		return nil, s.SerializeErr
	}
	return s.Serialized, nil
}

type Proposal struct {
	HeaderBytes  []byte
	PayloadBytes []byte
}

func (p *Proposal) Header() []byte  { return p.HeaderBytes }
func (p *Proposal) Payload() []byte { return p.PayloadBytes }

type ProposalResponse struct {
	EndorserBytes          []byte
	PayloadBytes           []byte
	EndorserSignatureBytes []byte
	ResultsBytes           []byte
	Status                 int32
	Message                string
}

func (r *ProposalResponse) Endorser() []byte          { return r.EndorserBytes }
func (r *ProposalResponse) Payload() []byte           { return r.PayloadBytes }
func (r *ProposalResponse) EndorserSignature() []byte { return r.EndorserSignatureBytes }
func (r *ProposalResponse) Results() []byte           { return r.ResultsBytes }
func (r *ProposalResponse) ResponseStatus() int32     { return r.Status }
func (r *ProposalResponse) ResponseMessage() string   { return r.Message }
func (r *ProposalResponse) Bytes() ([]byte, error)    { return nil, nil }
func (r *ProposalResponse) VerifyEndorsement(driver.VerifierProvider) error {
	return nil
}
