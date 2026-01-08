/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/protoutil"
)

// Signer is the interface needed to sign a transaction
type Signer interface {
	Sign(msg []byte) ([]byte, error)
	Serialize() ([]byte, error)
}

func GetEnvelopeFromBlock(data []byte) (*common.Envelope, error) {
	return protoutil.GetEnvelopeFromBlock(data)
}

// UnmarshalPayload unmarshals bytes to a Payload
func UnmarshalPayload(encoded []byte) (*common.Payload, error) {
	return protoutil.UnmarshalPayload(encoded)
}

// UnmarshalProposal unmarshals bytes to a Proposal
func UnmarshalProposal(propBytes []byte) (*peer.Proposal, error) {
	return protoutil.UnmarshalProposal(propBytes)
}

// UnmarshalProposalResponsePayload unmarshals bytes to a ProposalResponsePayload
func UnmarshalProposalResponsePayload(prpBytes []byte) (*peer.ProposalResponsePayload, error) {
	return protoutil.UnmarshalProposalResponsePayload(prpBytes)
}

// UnmarshalChaincodeAction unmarshals bytes to a ChaincodeAction
func UnmarshalChaincodeAction(caBytes []byte) (*peer.ChaincodeAction, error) {
	return protoutil.UnmarshalChaincodeAction(caBytes)
}

// UnmarshalEnvelope unmarshals bytes to a Envelope
func UnmarshalEnvelope(encoded []byte) (*common.Envelope, error) {
	return protoutil.UnmarshalEnvelope(encoded)
}

// UnmarshalHeader unmarshals bytes to a Header
func UnmarshalHeader(bytes []byte) (*common.Header, error) {
	return protoutil.UnmarshalHeader(bytes)
}

// UnmarshalChaincodeProposalPayload unmarshals bytes to a ChaincodeProposalPayload
func UnmarshalChaincodeProposalPayload(bytes []byte) (*peer.ChaincodeProposalPayload, error) {
	return protoutil.UnmarshalChaincodeProposalPayload(bytes)
}

// UnmarshalSerializedIdentity unmarshals bytes to a SerializedIdentity
func UnmarshalSerializedIdentity(bytes []byte) (*msp.SerializedIdentity, error) {
	return protoutil.UnmarshalSerializedIdentity(bytes)
}

// UnmarshalSignatureHeader unmarshals bytes to a SignatureHeader
func UnmarshalSignatureHeader(bytes []byte) (*common.SignatureHeader, error) {
	return protoutil.UnmarshalSignatureHeader(bytes)
}

// UnmarshalChaincodeInvocationSpec unmarshals bytes to a ChaincodeInvocationSpec
func UnmarshalChaincodeInvocationSpec(encoded []byte) (*peer.ChaincodeInvocationSpec, error) {
	return protoutil.UnmarshalChaincodeInvocationSpec(encoded)
}

// UnmarshalChannelHeader unmarshals bytes to a ChannelHeader
func UnmarshalChannelHeader(bytes []byte) (*common.ChannelHeader, error) {
	return protoutil.UnmarshalChannelHeader(bytes)
}

// UnmarshalChaincodeActionPayload unmarshals bytes to a ChaincodeActionPayload
func UnmarshalChaincodeActionPayload(capBytes []byte) (*peer.ChaincodeActionPayload, error) {
	return protoutil.UnmarshalChaincodeActionPayload(capBytes)
}

// GetBytesProposalPayloadForTx takes a ChaincodeProposalPayload and returns
// its serialized version according to the visibility field
func GetBytesProposalPayloadForTx(
	payload *peer.ChaincodeProposalPayload,
) ([]byte, error) {
	return protoutil.GetBytesProposalPayloadForTx(payload)
}

// GetBytesChaincodeActionPayload get the bytes of ChaincodeActionPayload from
// the message
func GetBytesChaincodeActionPayload(cap *peer.ChaincodeActionPayload) ([]byte, error) {
	return protoutil.GetBytesChaincodeActionPayload(cap)
}

// GetBytesTransaction get the bytes of Transaction from the message
func GetBytesTransaction(tx *peer.Transaction) ([]byte, error) {
	return protoutil.GetBytesTransaction(tx)
}

// GetBytesPayload get the bytes of Payload from the message
func GetBytesPayload(payl *common.Payload) ([]byte, error) {
	return protoutil.GetBytesPayload(payl)
}

// UnmarshalTransaction unmarshals bytes to a Transaction
func UnmarshalTransaction(txBytes []byte) (*peer.Transaction, error) {
	return protoutil.UnmarshalTransaction(txBytes)
}

// GetPayloads gets the underlying payload objects in a TransactionAction
func GetPayloads(txActions *peer.TransactionAction) (*peer.ChaincodeActionPayload, *peer.ChaincodeAction, error) {
	return protoutil.GetPayloads(txActions)
}

// UnmarshalChaincodeHeaderExtension unmarshals bytes to a ChaincodeHeaderExtension
func UnmarshalChaincodeHeaderExtension(hdrExtension []byte) (*peer.ChaincodeHeaderExtension, error) {
	return protoutil.UnmarshalChaincodeHeaderExtension(hdrExtension)
}

// UnmarshalChaincodeEvents unmarshals bytes to a ChaincodeEvent
func UnmarshalChaincodeEvents(eBytes []byte) (*peer.ChaincodeEvent, error) {
	return protoutil.UnmarshalChaincodeEvents(eBytes)
}

// CreateSignedTx assembles an Envelope message from proposal, endorsements,
// and a signer. This function should be called by a client when it has
// collected enough endorsements for a proposal to create a transaction and
// submit it to peers for ordering
func CreateSignedTx(
	proposal *peer.Proposal,
	signer Signer,
	resps ...*peer.ProposalResponse,
) (*common.Envelope, error) {
	return protoutil.CreateSignedTx(proposal, signer, resps...)
}

// GetSignedProposal returns a signed proposal given a Proposal message and a
// signing identity
func GetSignedProposal(prop *peer.Proposal, signer Signer) (*peer.SignedProposal, error) {
	return protoutil.GetSignedProposal(prop, signer)
}

// CreateChaincodeProposalWithTxIDNonceAndTransient creates a proposal from
// given input
func CreateChaincodeProposalWithTxIDNonceAndTransient(
	txid string,
	typ common.HeaderType,
	channelID string,
	cis *peer.ChaincodeInvocationSpec,
	nonce, creator []byte,
	transientMap map[string][]byte,
) (*peer.Proposal, string, error) {
	return protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(txid, typ, channelID, cis, nonce, creator, transientMap)
}

// GetBytesProposalResponsePayload gets proposal response payload
func GetBytesProposalResponsePayload(hash []byte, response *peer.Response, result []byte, event []byte, ccid *peer.ChaincodeID) ([]byte, error) {
	return protoutil.GetBytesProposalResponsePayload(hash, response, result, event, ccid)
}

// CreateSignedEnvelope creates a signed envelope of the desired type, with
// marshaled dataMsg and signs it
func CreateSignedEnvelope(
	txType common.HeaderType,
	channelID string,
	signer Signer,
	dataMsg proto.Message,
	msgVersion int32,
	epoch uint64,
) (*common.Envelope, error) {
	return protoutil.CreateSignedEnvelope(txType, channelID, signer, dataMsg, msgVersion, epoch)
}

// MarshalOrPanic serializes a protobuf message and panics if this
// operation fails
func MarshalOrPanic(pb proto.Message) []byte {
	return protoutil.MarshalOrPanic(pb)
}

// Marshal serializes a protobuf message.
func Marshal(pb proto.Message) ([]byte, error) {
	return protoutil.Marshal(pb)
}

// UnmarshalBlock unmarshals bytes to a Block
func UnmarshalBlock(encoded []byte) (*common.Block, error) {
	return protoutil.UnmarshalBlock(encoded)
}

// ComputeTxID computes TxID as the Hash computed
// over the concatenation of nonce and creator.
func ComputeTxID(nonce, creator []byte) string {
	return protoutil.ComputeTxID(nonce, creator)
}
