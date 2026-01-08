/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/sha256"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric/core/common/ccprovider"
	"github.com/hyperledger/fabric/msp"
)

// UnpackedProposal contains the interesting artifacts from inside the proposal.
type UnpackedProposal struct {
	ChaincodeName    string
	ChaincodeVersion string
	ChannelHeader    *cb.ChannelHeader
	Input            *pb.ChaincodeInput
	Proposal         *pb.Proposal
	SignatureHeader  *cb.SignatureHeader
	SignedProposal   *pb.SignedProposal
	ProposalHash     []byte
}

func (up *UnpackedProposal) ChannelID() string {
	return up.ChannelHeader.ChannelId
}

func (up *UnpackedProposal) TxID() string {
	return up.ChannelHeader.TxId
}

func (up *UnpackedProposal) Nonce() []byte {
	return up.SignatureHeader.Nonce
}

// UnpackSignedProposal creates an an *UnpackedProposal which is guaranteed to have
// no zero-ed fields or it returns an error.
func UnpackSignedProposal(signedProp *pb.SignedProposal) (*UnpackedProposal, error) {
	prop, err := protoutil.UnmarshalProposal(signedProp.ProposalBytes)
	if err != nil {
		return nil, err
	}

	up, err := UnpackProposal(prop)
	if err != nil {
		return nil, err
	}
	up.SignedProposal = signedProp
	return up, nil
}

func UnpackProposal(prop *pb.Proposal) (*UnpackedProposal, error) {
	hdr, err := protoutil.UnmarshalHeader(prop.Header)
	if err != nil {
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(hdr.ChannelHeader)
	if err != nil {
		return nil, err
	}

	shdr, err := protoutil.UnmarshalSignatureHeader(hdr.SignatureHeader)
	if err != nil {
		return nil, err
	}

	chaincodeHdrExt, err := protoutil.UnmarshalChaincodeHeaderExtension(chdr.Extension)
	if err != nil {
		return nil, err
	}

	if chaincodeHdrExt.ChaincodeId == nil {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId is nil")
	}

	if chaincodeHdrExt.ChaincodeId.Name == "" {
		return nil, errors.Errorf("ChaincodeHeaderExtension.ChaincodeId.Name is empty")
	}

	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(prop.Payload)
	if err != nil {
		return nil, err
	}

	cis, err := protoutil.UnmarshalChaincodeInvocationSpec(cpp.Input)
	if err != nil {
		return nil, err
	}

	if cis.ChaincodeSpec == nil {
		return nil, errors.Errorf("chaincode invocation spec did not contain chaincode spec")
	}

	if cis.ChaincodeSpec.Input == nil {
		return nil, errors.Errorf("chaincode input did not contain any input")
	}

	cppNoTransient := &pb.ChaincodeProposalPayload{Input: cpp.Input, TransientMap: nil}
	ppBytes, err := proto.Marshal(cppNoTransient)
	if err != nil {
		return nil, errors.WithMessage(err, "could not marshal non-transient portion of payload")
	}

	// TODO, this was preserved from the proputils stuff, but should this be BCCSP?

	// The proposal hash is the hash of the concatenation of:
	// 1) The serialized Channel Header object
	// 2) The serialized Signature Header object
	// 3) The hash of the part of the chaincode proposal payload that will go to the tx
	// (ie, the parts without the transient data)
	propHash := sha256.New()
	propHash.Write(hdr.ChannelHeader)
	propHash.Write(hdr.SignatureHeader)
	propHash.Write(ppBytes)

	return &UnpackedProposal{
		SignedProposal:   nil,
		Proposal:         prop,
		ChannelHeader:    chdr,
		SignatureHeader:  shdr,
		ChaincodeName:    chaincodeHdrExt.ChaincodeId.Name,
		ChaincodeVersion: chaincodeHdrExt.ChaincodeId.Version,
		Input:            cis.ChaincodeSpec.Input,
		ProposalHash:     propHash.Sum(nil)[:],
	}, nil
}

func (up *UnpackedProposal) Validate(idDeserializer msp.IdentityDeserializer) error {
	logger := decorateLogger(logger, &ccprovider.TransactionParams{
		ChannelID: up.ChannelHeader.ChannelId,
		TxID:      up.TxID(),
	})

	// validate the header type
	switch cb.HeaderType(up.ChannelHeader.Type) {
	case cb.HeaderType_ENDORSER_TRANSACTION:
	case cb.HeaderType_CONFIG:
		// The CONFIG transaction type has _no_ business coming to the propose API.
		// In fact, anything coming to the Propose API is by definition an endorser
		// transaction, so any other header type seems like it ought to be an error... oh well.

	default:
		return errors.Errorf("invalid header type %s", cb.HeaderType(up.ChannelHeader.Type))
	}

	// ensure the epoch is 0
	if up.ChannelHeader.Epoch != 0 {
		return errors.Errorf("epoch is non-zero")
	}

	// ensure that there is a nonce
	if len(up.SignatureHeader.Nonce) == 0 {
		return errors.Errorf("nonce is empty")
	}

	// ensure that there is a creator
	if len(up.SignatureHeader.Creator) == 0 {
		return errors.New("creator is empty")
	}

	expectedTxID := protoutil.ComputeTxID(up.SignatureHeader.Nonce, up.SignatureHeader.Creator)
	if up.TxID() != expectedTxID {
		return errors.Errorf("incorrectly computed txid '%s' -- expected '%s'", up.TxID(), expectedTxID)
	}

	if up.SignedProposal.ProposalBytes == nil {
		return errors.Errorf("empty proposal bytes")
	}

	if up.SignedProposal.Signature == nil {
		return errors.Errorf("empty signature bytes")
	}

	sID, err := protoutil.UnmarshalSerializedIdentity(up.SignatureHeader.Creator)
	if err != nil {
		return errors.Errorf("access denied: channel [%s] creator org unknown, creator is malformed", up.ChannelID())
	}

	genericAuthError := errors.Errorf("access denied: channel [%s] creator org [%s]", up.ChannelID(), sID.Mspid)

	// get the identity of the creator
	creator, err := idDeserializer.DeserializeIdentity(up.SignatureHeader.Creator)
	if err != nil {
		logger.Warningf("access denied: channel %s", err)
		return genericAuthError
	}

	// ensure that creator is a valid certificate
	err = creator.Validate()
	if err != nil {
		logger.Warningf("access denied: identity is not valid: %s", err)
		return genericAuthError
	}

	logger = logger.With("mspID", creator.GetMSPIdentifier())

	logger.Debug("creator is valid")

	// validate the signature
	err = creator.Verify(up.SignedProposal.ProposalBytes, up.SignedProposal.Signature)
	if err != nil {
		logger.Warningf("access denied: creator's signature over the proposal is not valid: %s", err)
		return genericAuthError
	}

	logger.Debug("signature is valid")

	return nil
}

func decorateLogger(logger logging.Logger, txParams *ccprovider.TransactionParams) logging.Logger {
	return logger.With("channel", txParams.ChannelID, "txID", shorttxid(txParams.TxID))
}

func shorttxid(txid string) string {
	if len(txid) < 8 {
		return txid
	}
	return txid[0:8]
}
