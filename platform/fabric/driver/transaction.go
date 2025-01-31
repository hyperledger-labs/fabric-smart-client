/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/common"
)

type TransactionType int32

const (
	EndorserTransaction = TransactionType(common.HeaderType_ENDORSER_TRANSACTION)
)

type Envelope interface {
	TxID() string
	Nonce() []byte
	Creator() []byte
	Results() []byte
	Bytes() ([]byte, error)
	FromBytes(raw []byte) error
	String() string
}

type ProposalResponse interface {
	Endorser() []byte
	Payload() []byte
	EndorserSignature() []byte
	Results() []byte
	ResponseStatus() int32
	ResponseMessage() string
	Bytes() ([]byte, error)
	VerifyEndorsement(provider VerifierProvider) error
}

type Proposal interface {
	Header() []byte
	Payload() []byte
}

type TransientMap map[string][]byte

type MetadataService interface {
	Exists(txid string) bool
	StoreTransient(txid string, transientMap TransientMap) error
	LoadTransient(txid string) (TransientMap, error)
}

type EnvelopeService interface {
	Exists(txid string) bool
	StoreEnvelope(txid string, env interface{}) error
	LoadEnvelope(txid string) ([]byte, error)
}

type EndorserTransactionService interface {
	Exists(txid string) bool
	StoreTransaction(txid string, raw []byte) error
	LoadTransaction(txid string) ([]byte, error)
}

type TransactionFactory interface {
	NewTransaction(ctx context.Context, channel string, nonce []byte, creator []byte, txid string, rawRequest []byte) (Transaction, error)
}

type TransactionManager interface {
	ComputeTxID(id *TxID) string
	NewEnvelope() Envelope
	NewProposalResponseFromBytes(raw []byte) (ProposalResponse, error)
	NewTransaction(ctx context.Context, transactionType TransactionType, creator view.Identity, nonce []byte, txid string, channel string, rawRequest []byte) (Transaction, error)
	NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (Transaction, error)
	NewTransactionFromEnvelopeBytes(ctx context.Context, channel string, raw []byte) (Transaction, error)
	AddTransactionFactory(tt TransactionType, factory TransactionFactory)
	NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (ProcessedTransaction, int32, error)
	NewProcessedTransactionFromEnvelopeRaw(envelope []byte) (ProcessedTransaction, error)
	NewProcessedTransaction(pt []byte) (ProcessedTransaction, error)
}

type Transaction interface {
	Creator() view.Identity
	Nonce() []byte
	ID() string
	Network() string
	Channel() string
	Function() string
	Parameters() [][]byte
	FunctionAndParameters() (string, []string)
	Chaincode() string
	ChaincodeVersion() string
	Results() ([]byte, error)
	From(payload Transaction) (err error)
	SetFromBytes(raw []byte) error
	SetFromEnvelopeBytes(raw []byte) error
	Proposal() Proposal
	SignedProposal() SignedProposal
	SetProposal(chaincode string, version string, function string, params ...string)
	AppendParameter(p []byte)
	SetParameterAt(i int, p []byte) error
	Transient() TransientMap
	ResetTransient()
	SetRWSet() error
	RWS() RWSet
	Done() error
	Close()
	Raw() ([]byte, error)
	GetRWSet() (RWSet, error)
	Bytes() ([]byte, error)
	Endorse() error
	EndorseWithIdentity(identity view.Identity) error
	EndorseWithSigner(identity view.Identity, s Signer) error
	EndorseProposal() error
	EndorseProposalWithIdentity(identity view.Identity) error
	EndorseProposalResponse() error
	EndorseProposalResponseWithIdentity(identity view.Identity) error
	AppendProposalResponse(response ProposalResponse) error
	ProposalHasBeenEndorsedBy(party view.Identity) error
	StoreTransient() error
	ProposalResponses() ([]ProposalResponse, error)
	ProposalResponse() ([]byte, error)
	BytesNoTransient() ([]byte, error)
	Envelope() (Envelope, error)
}

type SignedProposal interface {
	ProposalBytes() []byte
	Signature() []byte
	ProposalHash() []byte
	ChaincodeName() string
	ChaincodeVersion() string
}
