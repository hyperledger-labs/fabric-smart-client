/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"bytes"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Transaction struct {
	view2.Provider

	Transaction       *fabric.Transaction
	verifierProviders []fabric.VerifierProvider
}

func (t *Transaction) ID() string {
	return t.Transaction.ID()
}

func (t *Transaction) Network() string {
	return t.Transaction.Network()
}

func (t *Transaction) Channel() string {
	return t.Transaction.Channel()
}

func (t *Transaction) FunctionAndParameters() (string, []string) {
	var params []string
	for _, parameter := range t.Transaction.Parameters() {
		params = append(params, string(parameter))
	}

	return t.Transaction.Function(), params
}

func (t *Transaction) SetProposal(chaincode string, version string, function string, params ...string) {
	t.Transaction.SetProposal(chaincode, version, function, params...)
}

func (t *Transaction) Chaincode() (string, string) {
	return t.Transaction.Chaincode(), t.Transaction.ChaincodeVersion()
}

func (t *Transaction) Parameters() [][]byte {
	return t.Transaction.Parameters()
}

func (t *Transaction) AppendParameter(p []byte) {
	t.Transaction.AppendParameter(p)
}

func (t *Transaction) SetParameterAt(i int, p []byte) error {
	return t.Transaction.SetParameterAt(i, p)
}

func (t *Transaction) RWSet() (*fabric.RWSet, error) {
	return t.Transaction.GetRWSet()
}

func (t *Transaction) Results() ([]byte, error) {
	rwset, err := t.RWSet()
	if err != nil {
		return nil, err
	}
	defer rwset.Done()
	return rwset.Bytes()
}

func (t *Transaction) Hash() ([]byte, error) {
	raw, err := t.Bytes()
	if err != nil {
		return nil, err
	}
	return hash.SHA256(raw)
}

func (t *Transaction) SetFromBytes(raw []byte) error {
	return t.Transaction.SetFromBytes(raw)
}

func (t *Transaction) SetFromEnvelopeBytes(raw []byte) error {
	return t.Transaction.SetFromEnvelopeBytes(raw)
}

func (t *Transaction) Bytes() ([]byte, error) {
	return t.Transaction.Bytes()
}

func (t *Transaction) BytesNoTransient() ([]byte, error) {
	return t.Transaction.BytesNoTransient()
}

func (t *Transaction) Raw() ([]byte, error) {
	return t.Transaction.Raw()
}

func (t *Transaction) Creator() view.Identity {
	return t.Transaction.Creator()
}

func (t *Transaction) Endorse() error {
	return t.Transaction.Endorse()
}

func (t *Transaction) EndorseWithIdentity(identity view.Identity) error {
	return t.Transaction.EndorseWithIdentity(identity)
}

func (t *Transaction) EndorseWithSigner(identity view.Identity, s fabric.Signer) error {
	return t.Transaction.EndorseWithSigner(identity, s)
}

func (t *Transaction) EndorseProposal() error {
	return t.Transaction.EndorseProposal()
}

func (t *Transaction) EndorseProposalWithIdentity(identity view.Identity) error {
	return t.Transaction.EndorseProposalWithIdentity(identity)
}

func (t *Transaction) EndorseProposalResponse() error {
	return t.Transaction.EndorseProposalResponse()
}

func (t *Transaction) EndorseProposalResponseWithIdentity(identity view.Identity) error {
	return t.Transaction.EndorseProposalResponseWithIdentity(identity)
}

func (t *Transaction) ProposalResponse() ([]byte, error) {
	return t.Transaction.ProposalResponse()
}

func (t *Transaction) AppendProposalResponse(response *fabric.ProposalResponse) error {
	return t.Transaction.AppendProposalResponse(response)
}

// HasBeenEndorsedBy returns nil if each passed party has signed this transaction
func (t *Transaction) HasBeenEndorsedBy(parties ...view.Identity) error {
	responses, err := t.Transaction.ProposalResponses()
	if err != nil {
		return err
	}

	for _, party := range parties {
		found := false
		for _, response := range responses {
			if bytes.Equal(response.Endorser(), party) {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("party [%s] has not signed", party)
		}
	}
	return nil
}

func (t *Transaction) GetSignatureOf(party view.Identity) ([]byte, error) {
	responses, err := t.Transaction.ProposalResponses()
	if err != nil {
		return nil, err
	}
	for _, response := range responses {
		if bytes.Equal(response.Endorser(), party) {
			return response.EndorserSignature(), nil
		}
	}
	return nil, nil
}

// Namespaces returns the list of namespaces this transaction contains.
func (t *Transaction) Namespaces() Namespaces {
	rwSet, err := t.RWSet()
	if err != nil {
		panic(errors.Wrap(err, "filed getting rw set").Error())
	}

	return rwSet.Namespaces()
}

func (t *Transaction) Close() {
	t.Transaction.Close()
}

func (t *Transaction) SetTransient(key string, raw []byte) error {
	transient := t.Transaction.Transient()
	return transient.Set(key, raw)
}

func (t *Transaction) GetTransient(key string) []byte {
	transient := t.Transaction.Transient()
	return transient.Get(key)
}

func (t *Transaction) SetTransientState(key string, state interface{}) error {
	transient := t.Transaction.Transient()
	return transient.SetState(key, state)
}

func (t *Transaction) GetTransientState(key string, state interface{}) error {
	transient := t.Transaction.Transient()
	return transient.GetState(key, state)
}

func (t *Transaction) ExistsTransientState(key string) bool {
	transient := t.Transaction.Transient()
	return transient.Exists(key)
}

func (t *Transaction) FabricNetworkService() *fabric.NetworkService {
	return t.Transaction.FabricNetworkService()
}

func (t *Transaction) AppendVerifierProvider(vp fabric.VerifierProvider) {
	t.verifierProviders = append(t.verifierProviders, vp)
}

func (t *Transaction) Envelope() (*fabric.Envelope, error) {
	return t.Transaction.Envelope()
}

func (t *Transaction) ProposalResponses() ([][]byte, error) {
	prs, err := t.Transaction.ProposalResponses()
	if err != nil {
		return nil, err
	}
	res := [][]byte{}
	for _, pr := range prs {
		prRaw, err := pr.Bytes()
		if err != nil {
			return nil, err
		}
		res = append(res, prRaw)
	}
	return res, nil
}
