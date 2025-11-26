/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type VerifierProvider = driver.VerifierProvider

type TransactionType = driver.TransactionType

const (
	EndorserTransaction = driver.EndorserTransaction
)

type TransactionOptions struct {
	Creator         view.Identity
	Nonce           []byte
	TxID            string
	Channel         string
	RawRequest      []byte
	TransactionType TransactionType
	Context         context.Context
}

type TransactionOption func(*TransactionOptions) error

func CompileTransactionOptions(opts ...TransactionOption) (*TransactionOptions, error) {
	txOptions := &TransactionOptions{
		TransactionType: EndorserTransaction,
	}
	for _, opt := range opts {
		if err := opt(txOptions); err != nil {
			return nil, err
		}
	}
	return txOptions, nil
}

func WithCreator(creator view.Identity) TransactionOption {
	return func(o *TransactionOptions) error {
		o.Creator = creator
		return nil
	}
}

func WithContext(ctx context.Context) TransactionOption {
	return func(o *TransactionOptions) error {
		o.Context = ctx
		return nil
	}
}

func WithChannel(channel string) TransactionOption {
	return func(o *TransactionOptions) error {
		o.Channel = channel
		return nil
	}
}

func WithNonce(nonce []byte) TransactionOption {
	return func(o *TransactionOptions) error {
		o.Nonce = nonce
		return nil
	}
}

func WithTxID(txid string) TransactionOption {
	return func(o *TransactionOptions) error {
		o.TxID = txid
		return nil
	}
}

func WithRawRequest(rawRequest []byte) TransactionOption {
	return func(o *TransactionOptions) error {
		o.RawRequest = rawRequest
		return nil
	}
}

func WithTransactionType(tt TransactionType) TransactionOption {
	return func(o *TransactionOptions) error {
		o.TransactionType = tt
		return nil
	}
}

type TxID struct {
	Nonce   []byte
	Creator []byte
}

func (t *TxID) String() string {
	return fmt.Sprintf("[%s:%s]", base64.StdEncoding.EncodeToString(t.Nonce), base64.StdEncoding.EncodeToString(t.Creator))
}

type ProposalResponse struct {
	pr driver.ProposalResponse
}

// NewProposalResponse returns a new instance of ProposalResponse for the passed arguments
func NewProposalResponse(pr driver.ProposalResponse) *ProposalResponse {
	return &ProposalResponse{pr: pr}
}

func (r *ProposalResponse) ResponseStatus() int32 {
	return r.pr.ResponseStatus()
}

func (r *ProposalResponse) ResponseMessage() string {
	return r.pr.ResponseMessage()
}

func (r *ProposalResponse) Endorser() []byte {
	return r.pr.Endorser()
}

func (r *ProposalResponse) Payload() []byte {
	return r.pr.Payload()
}

func (r *ProposalResponse) EndorserSignature() []byte {
	return r.pr.EndorserSignature()
}

func (r *ProposalResponse) Results() []byte {
	return r.pr.Results()
}

func (r *ProposalResponse) Bytes() ([]byte, error) {
	return r.pr.Bytes()
}

func (r *ProposalResponse) VerifyEndorsement(provider VerifierProvider) error {
	return r.pr.VerifyEndorsement(provider)
}

type Proposal struct {
	p driver.Proposal
}

func (p *Proposal) Header() []byte {
	return p.p.Header()
}

func (p *Proposal) Payload() []byte {
	return p.p.Payload()
}

type SignedProposal struct {
	s driver.SignedProposal
}

func (p *SignedProposal) ProposalBytes() []byte {
	return p.s.ProposalBytes()
}

func (p *SignedProposal) Signature() []byte {
	return p.s.Signature()
}

func (p *SignedProposal) ProposalHash() []byte {
	return p.s.ProposalHash()
}

func (p *SignedProposal) ChaincodeName() string {
	return p.s.ChaincodeName()
}

func (p *SignedProposal) ChaincodeVersion() string {
	return p.s.ChaincodeVersion()
}

type Signer interface {
	Sign(message []byte) ([]byte, error)
}

type Transaction struct {
	fns *NetworkService
	tx  driver.Transaction
}

// NewTransaction returns a new instance of Transaction for the given arguments
func NewTransaction(fns *NetworkService, tx driver.Transaction) *Transaction {
	return &Transaction{fns: fns, tx: tx}
}

func (t *Transaction) Creator() view.Identity {
	return t.tx.Creator()
}

func (t *Transaction) Nonce() []byte {
	return t.tx.Nonce()
}

func (t *Transaction) ID() string {
	return t.tx.ID()
}

func (t *Transaction) Network() string {
	return t.tx.Network()
}

func (t *Transaction) Channel() string {
	return t.tx.Channel()
}

func (t *Transaction) Function() string {
	return t.tx.Function()
}

func (t *Transaction) Parameters() [][]byte {
	return t.tx.Parameters()
}

func (t *Transaction) Chaincode() string {
	return t.tx.Chaincode()
}

func (t *Transaction) ChaincodeVersion() string {
	return t.tx.ChaincodeVersion()
}

func (t *Transaction) Results() ([]byte, error) {
	return t.tx.Results()
}

func (t *Transaction) From(payload *Transaction) (err error) {
	return t.tx.From(payload.tx)
}

func (t *Transaction) SetFromBytes(raw []byte) error {
	return t.tx.SetFromBytes(raw)
}

func (t *Transaction) SetFromEnvelopeBytes(raw []byte) error {
	return t.tx.SetFromEnvelopeBytes(raw)
}

func (t *Transaction) Proposal() *Proposal {
	return &Proposal{t.tx.Proposal()}
}

func (t *Transaction) SignedProposal() *SignedProposal {
	return &SignedProposal{s: t.tx.SignedProposal()}
}

func (t *Transaction) SetProposal(chaincode string, version string, function string, params ...string) {
	t.tx.SetProposal(chaincode, version, function, params...)
}

func (t *Transaction) AppendParameter(p []byte) {
	t.tx.AppendParameter(p)
}

func (t *Transaction) SetParameterAt(i int, p []byte) error {
	return t.tx.SetParameterAt(i, p)
}

func (t *Transaction) Transient() TransientMap {
	return TransientMap(t.tx.Transient())
}

func (t *Transaction) ResetTransient() {
	t.tx.ResetTransient()
}

func (t *Transaction) SetRWSet() error {
	return t.tx.SetRWSet()
}

func (t *Transaction) RWS() *RWSet {
	return NewRWSet(t.tx.RWS())
}

func (t *Transaction) Done() error {
	return t.tx.Done()
}

func (t *Transaction) Close() {
	t.tx.Close()
}

func (t *Transaction) Raw() ([]byte, error) {
	return t.tx.Raw()
}

func (t *Transaction) GetRWSet() (*RWSet, error) {
	rws, err := t.tx.GetRWSet()
	if err != nil {
		return nil, err
	}
	return NewRWSet(rws), nil
}

func (t *Transaction) Bytes() ([]byte, error) {
	return t.tx.Bytes()
}

func (t *Transaction) Endorse() error {
	return t.tx.Endorse()
}

func (t *Transaction) EndorseWithIdentity(identity view.Identity) error {
	return t.tx.EndorseWithIdentity(identity)
}

func (t *Transaction) EndorseWithSigner(identity view.Identity, s Signer) error {
	return t.tx.EndorseWithSigner(identity, s)
}

func (t *Transaction) EndorseProposal() error {
	return t.tx.EndorseProposal()
}

func (t *Transaction) EndorseProposalWithIdentity(identity view.Identity) error {
	return t.tx.EndorseProposalWithIdentity(identity)
}

func (t *Transaction) EndorseProposalResponse() error {
	return t.tx.EndorseProposalResponse()
}

func (t *Transaction) EndorseProposalResponseWithIdentity(identity view.Identity) error {
	return t.tx.EndorseProposalResponseWithIdentity(identity)
}

func (t *Transaction) AppendProposalResponse(response *ProposalResponse) error {
	return t.tx.AppendProposalResponse(response.pr)
}

func (t *Transaction) ProposalHasBeenEndorsedBy(party view.Identity) error {
	return t.tx.ProposalHasBeenEndorsedBy(party)
}

func (t *Transaction) StoreTransient() error {
	return t.tx.StoreTransient()
}

func (t *Transaction) ProposalResponses() ([]*ProposalResponse, error) {
	var res []*ProposalResponse
	responses, err := t.tx.ProposalResponses()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to fetch proposal responses")
	}
	for _, resp := range responses {
		res = append(res, &ProposalResponse{pr: resp})
	}
	return res, nil
}

func (t *Transaction) ProposalResponse() ([]byte, error) {
	return t.tx.ProposalResponse()
}

func (t *Transaction) BytesNoTransient() ([]byte, error) {
	return t.tx.BytesNoTransient()
}

func (t *Transaction) FabricNetworkService() *NetworkService {
	return t.fns
}

func (t *Transaction) Envelope() (*Envelope, error) {
	env, err := t.tx.Envelope()
	if err != nil {
		return nil, err
	}
	return NewEnvelope(env), nil
}

type TransactionManager struct {
	fns *NetworkService
}

func (t *TransactionManager) NewEnvelope() *Envelope {
	return NewEnvelope(t.fns.fns.TransactionManager().NewEnvelope())
}

func (t *TransactionManager) NewProposalResponseFromBytes(raw []byte) (*ProposalResponse, error) {
	pr, err := t.fns.fns.TransactionManager().NewProposalResponseFromBytes(raw)
	if err != nil {
		return nil, err
	}
	return &ProposalResponse{pr: pr}, nil
}

func (t *TransactionManager) NewTransaction(opts ...TransactionOption) (*Transaction, error) {
	options, err := CompileTransactionOptions(opts...)
	if err != nil {
		return nil, err
	}
	ch, err := t.fns.Channel(options.Channel)
	if err != nil {
		return nil, err
	}

	tx, err := t.fns.fns.TransactionManager().NewTransaction(options.Context, driver.TransactionType(options.TransactionType), options.Creator, options.Nonce, options.TxID, ch.Name(), options.RawRequest)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		fns: t.fns,
		tx:  tx,
	}, nil
}

func (t *TransactionManager) NewTransactionFromBytes(raw []byte, opts ...TransactionOption) (*Transaction, error) {
	options, err := CompileTransactionOptions(opts...)
	if err != nil {
		return nil, err
	}
	ch, err := t.fns.Channel(options.Channel)
	if err != nil {
		return nil, err
	}

	tx, err := t.fns.fns.TransactionManager().NewTransactionFromBytes(options.Context, ch.Name(), raw)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		fns: t.fns,
		tx:  tx,
	}, nil
}

func (t *TransactionManager) NewTransactionFromEnvelopeBytes(raw []byte, opts ...TransactionOption) (*Transaction, error) {
	options, err := CompileTransactionOptions(opts...)
	if err != nil {
		return nil, err
	}
	ch, err := t.fns.Channel(options.Channel)
	if err != nil {
		return nil, err
	}

	tx, err := t.fns.fns.TransactionManager().NewTransactionFromEnvelopeBytes(options.Context, ch.Name(), raw)
	if err != nil {
		return nil, err
	}

	return &Transaction{
		fns: t.fns,
		tx:  tx,
	}, nil
}

func (t *TransactionManager) ComputeTxID(id *TxID) string {
	txID := &driver.TxIDComponents{
		Nonce: id.Nonce, Creator: id.Creator,
	}
	res := t.fns.fns.TransactionManager().ComputeTxID(txID)
	id.Nonce = txID.Nonce
	id.Creator = txID.Creator
	return res
}

type MetadataService struct {
	ms driver.MetadataService
}

func (m *MetadataService) Exists(ctx context.Context, txid string) bool {
	return m.ms.Exists(ctx, txid)
}

func (m *MetadataService) StoreTransient(ctx context.Context, txid string, transientMap TransientMap) error {
	return m.ms.StoreTransient(ctx, txid, driver.TransientMap(transientMap))
}

func (m *MetadataService) LoadTransient(ctx context.Context, txid string) (TransientMap, error) {
	res, err := m.ms.LoadTransient(ctx, txid)
	if err != nil {
		return nil, err
	}
	return TransientMap(res), nil
}

type EnvelopeService struct {
	ms driver.EnvelopeService
}

func (m *EnvelopeService) Exists(ctx context.Context, txid string) bool {
	return m.ms.Exists(ctx, txid)
}
