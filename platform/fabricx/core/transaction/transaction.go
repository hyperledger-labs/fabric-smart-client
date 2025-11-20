/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-committer/utils/signature"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

var logger = logging.MustGetLogger()

type Transaction struct {
	ctx   context.Context
	fns   driver.FabricNetworkService
	rwset driver.RWSet

	// TODO: remove channel and use fns(Channel)
	channel driver.Channel

	signedProposal   *SignedProposal
	proposalResponse *pb.ProposalResponse

	TCreator view.Identity
	TNonce   []byte
	TTxID    string

	TNetwork          string
	TChannel          string
	TChaincode        string
	TChaincodeVersion string
	TFunction         string
	TParameters       [][]byte

	RWSet      []byte
	TTransient driver.TransientMap

	TProposal          *pb.Proposal
	TSignedProposal    *pb.SignedProposal
	TProposalResponses []*pb.ProposalResponse
}

func (t *Transaction) Creator() view.Identity {
	return t.TCreator
}

func (t *Transaction) Nonce() []byte {
	return t.TNonce
}

func (t *Transaction) ID() string {
	return t.TTxID
}

func (t *Transaction) Network() string {
	return t.TNetwork
}

func (t *Transaction) Channel() string {
	return t.TChannel
}

func (t *Transaction) Function() string {
	return t.TFunction
}

func (t *Transaction) Parameters() [][]byte {
	return t.TParameters
}

func (t *Transaction) FunctionAndParameters() (string, []string) {
	params := make([]string, len(t.Parameters()))
	for i, p := range t.Parameters() {
		params[i] = string(p)
	}

	return t.Function(), params
}

func (t *Transaction) Chaincode() string {
	return t.TChaincode
}

func (t *Transaction) ChaincodeVersion() string {
	return t.TChaincodeVersion
}

func (t *Transaction) Results() ([]byte, error) {
	return t.TProposalResponses[0].Payload, nil
}

func (t *Transaction) From(tx driver.Transaction) (err error) {
	payload, ok := tx.(*Transaction)
	if !ok {
		return fmt.Errorf("wrong transaction type [%T]", tx)
	}

	t.ctx = context.Background()
	t.TCreator = payload.TCreator
	t.TNonce = payload.TNonce
	t.TTxID = payload.TTxID
	t.TCreator = payload.TCreator
	t.TNetwork = payload.TNetwork
	t.TChannel = payload.TChannel
	t.TChaincode = payload.TChaincode
	t.TChaincodeVersion = payload.TChaincodeVersion
	t.TFunction = payload.TFunction
	t.TParameters = payload.TParameters
	t.RWSet = payload.RWSet
	t.TProposal = payload.TProposal
	t.TSignedProposal = payload.TSignedProposal
	if payload.TSignedProposal != nil {
		t.signedProposal, err = newSignedProposal(payload.TSignedProposal)
		if err != nil {
			return
		}
	}
	t.TProposalResponses = payload.TProposalResponses
	t.TTransient = payload.TTransient
	return
}

func (t *Transaction) SetFromBytes(raw []byte) error {
	if err := json.Unmarshal(raw, t); err != nil {
		return fmt.Errorf("SetFromBytes: failed unmarshalling payload [%s]: %w", string(raw), err)
	}

	if t.TSignedProposal != nil {
		// TODO: check the current payload is compatible with the content of the signed proposal
		up, err := transaction.UnpackSignedProposal(t.TSignedProposal)
		if err != nil {
			return fmt.Errorf("SetFromBytes: failed unpacking proposal [%s]: %w", string(raw), err)
		}
		t.TTxID = up.TxID()
		t.TNonce = up.Nonce()
		t.TChaincode = up.ChaincodeName
		t.TChaincodeVersion = up.ChaincodeVersion
		t.TFunction = string(up.Input.Args[0])
		t.TParameters = [][]byte{}
		for i := 1; i < len(up.Input.Args); i++ {
			t.TParameters = append(t.TParameters, up.Input.Args[i])
		}
		t.TChannel = up.ChannelHeader.ChannelId
		t.TProposal = up.Proposal
		if len(t.TCreator) == 0 {
			t.TCreator = up.SignatureHeader.Creator
		}
		t.signedProposal, err = newSignedProposal(t.TSignedProposal)
		if err != nil {
			return err
		}
	}

	// Set the channel
	ch, err := t.fns.Channel(t.Channel())
	if err != nil {
		return err
	}
	t.channel = ch

	return nil
}

func (t *Transaction) SetFromEnvelopeBytes(raw []byte) error {
	// TODO: check the current payload is compatible with the content of the signed proposal
	upe, _, err := transaction.UnpackEnvelopeFromBytes(raw)
	if err != nil {
		return err
	}

	t.TTxID = upe.TxID
	t.TNonce = upe.Nonce
	t.TChannel = upe.Ch
	t.TChaincode = upe.ChaincodeName
	t.TChaincodeVersion = upe.ChaincodeVersion
	t.TFunction = string(upe.Input.Args[0])
	t.TParameters = [][]byte{}
	for i := 1; i < len(upe.Input.Args); i++ {
		t.TParameters = append(t.TParameters, upe.Input.Args[i])
	}
	t.TChannel = upe.ChannelHeader.ChannelId
	if len(t.TCreator) == 0 {
		t.TCreator = upe.SignatureHeader.Creator
	}
	t.TProposalResponses = upe.ProposalResponses

	// Set the channel
	ch, err := t.fns.Channel(t.Channel())
	if err != nil {
		return err
	}
	t.channel = ch

	return nil
}

func (t *Transaction) Proposal() driver.Proposal {
	return &Proposal{
		p: t.TProposal,
	}
}

func (t *Transaction) SignedProposal() driver.SignedProposal {
	if t.signedProposal == nil {
		return nil
	}
	return t.signedProposal
}

func (t *Transaction) SetProposal(chaincode, version, function string, params ...string) {
	t.TChaincode = chaincode
	t.TChaincodeVersion = version
	t.TFunction = function

	t.TParameters = [][]byte{}
	for _, param := range params {
		t.TParameters = append(t.TParameters, []byte(param))
	}
}

func (t *Transaction) AppendParameter(p []byte) {
	t.TParameters = append(t.TParameters, p)
}

func (t *Transaction) SetParameterAt(i int, p []byte) error {
	if i >= len(t.TParameters) {
		return fmt.Errorf("invalid index, got [%d]>=[%d]", i, len(t.TParameters))
	}
	t.TParameters[i] = p
	return nil
}

func (t *Transaction) Transient() driver.TransientMap {
	return t.TTransient
}

func (t *Transaction) ResetTransient() {
	t.TTransient = map[string][]byte{}
}

func (t *Transaction) SetRWSet() error {
	span := trace.SpanFromContext(t.ctx)
	span.AddEvent("start_set_rwset")
	defer span.AddEvent("end_set_rwset")
	logger.Debugf("SetRWSet for transaction [%s]", t.ID())
	switch {
	case len(t.TProposalResponses) != 0:
		logger.Debugf("populate rws from proposal response")
		results, err := t.Results()
		if err != nil {
			return fmt.Errorf("failed to get rws from proposal response: %w", err)
		}
		t.rwset, err = t.channel.Vault().NewRWSetFromBytes(t.ctx, t.ID(), results)
		if err != nil {
			return fmt.Errorf("failed to populate rws from proposal response: %w", err)
		}
	case len(t.RWSet) != 0:
		logger.Debugf("populate rws from rwset")
		var err error
		t.rwset, err = t.channel.Vault().NewRWSetFromBytes(t.ctx, t.ID(), t.RWSet)
		if err != nil {
			return fmt.Errorf("failed to populate rws from existing rws: %w", err)
		}
	default:
		logger.Debugf("populate rws from scratch")
		var err error
		t.rwset, err = t.channel.Vault().NewRWSet(t.ctx, t.ID())
		if err != nil {
			return fmt.Errorf("failed to create fresh rws: %w", err)
		}
	}
	return nil
}

func (t *Transaction) RWS() driver.RWSet {
	return t.rwset
}

func (t *Transaction) Done() error {
	logger.Debugf("transaction [%s] done [%v]", t.ID(), t.rwset)
	if t.rwset != nil {
		// There is a simulation in progress:
		// 1. terminate it
		// 2. append it to the payload
		logger.Debugf("Call rwset done ...")
		t.rwset.Done()
		var err error
		t.RWSet, err = t.rwset.Bytes()
		if err != nil {
			return fmt.Errorf("failed marshalling rws: %w", err)
		}
		logger.Debugf("terminated simulation with [%s][len:%d]", t.rwset.Namespaces(), len(t.RWSet))
	}
	return nil
}

func (t *Transaction) Close() {
	logger.Debugf("closing transaction [%s,%v]", t.ID(), t.rwset != nil)
	if t.rwset != nil {
		t.rwset.Done()
		t.rwset = nil
	}
}

func (t *Transaction) Raw() ([]byte, error) {
	if t.rwset != nil {
		var err error
		t.RWSet, err = t.rwset.Bytes()
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(t)
}

func (t *Transaction) GetRWSet() (driver.RWSet, error) {
	if t.rwset == nil {
		err := t.SetRWSet()
		if err != nil {
			return nil, err
		}
	}
	return t.rwset, nil
}

func (t *Transaction) Bytes() ([]byte, error) {
	if err := t.Done(); err != nil {
		return nil, err
	}
	return json.Marshal(t)
}

func (t *Transaction) BytesNoTransient() ([]byte, error) {
	if err := t.Done(); err != nil {
		return nil, err
	}
	temp := &Transaction{}
	if err := temp.From(t); err != nil {
		return nil, err
	}
	temp.ResetTransient()
	return json.Marshal(temp)
}

func (t *Transaction) Endorse() error {
	return t.EndorseWithIdentity(t.Creator())
}

func (t *Transaction) EndorseWithIdentity(identity view.Identity) error {
	logger.Debugf("endorse transaction [%s] with identity [%s]", t.ID(), identity.String())

	// prepare signer
	s, err := t.fns.SignerService().GetSigner(identity)
	if err != nil {
		return err
	}
	signer := &signerWrapper{creator: identity, signer: s}

	return t.EndorseWithSigner(identity, signer)
}

func (t *Transaction) EndorseWithSigner(identity view.Identity, s driver.Signer) error {
	// prepare signer
	signer := &signerWrapper{creator: identity, signer: s}

	// is there a proposal already signed?
	if t.SignedProposal() == nil {
		// Endorse it
		if err := t.generateProposal(signer); err != nil {
			return fmt.Errorf("failed setting proposal: %w", err)
		}
	}

	// is there already a response or a simulation is in progress?
	if len(t.TProposalResponses) != 0 || len(t.RWSet) != 0 || t.RWS() != nil {
		logger.Debugf("endorse transaction [%s]", t.ID())
		defer t.Close()

		var err error
		t.proposalResponse, err = t.getProposalResponse(signer)
		if err != nil {
			return fmt.Errorf("failed getting proposal response: %w", err)
		}
		err = t.appendProposalResponse(t.proposalResponse)
		if err != nil {
			return fmt.Errorf("failed appending proposal response: %w", err)
		}
	}

	err := t.StoreTransient()
	if err != nil {
		return fmt.Errorf("failed storing transient: %w", err)
	}

	return nil
}

func (t *Transaction) EndorseProposal() error {
	return t.EndorseProposalWithIdentity(t.Creator())
}

func (t *Transaction) EndorseProposalWithIdentity(identity view.Identity) error {
	// prepare signer
	s, err := t.fns.SignerService().GetSigner(identity)
	if err != nil {
		return err
	}
	signer := &signerWrapper{creator: identity, signer: s}

	defer t.Close()
	if err := t.generateProposal(signer); err != nil {
		return err
	}

	return nil
}

func (t *Transaction) EndorseProposalResponse() error {
	return t.EndorseProposalResponseWithIdentity(t.Creator())
}

func (t *Transaction) EndorseProposalResponseWithIdentity(identity view.Identity) error {
	logger.Debugf("endorse with [%s]", identity)
	// prepare signer
	s, err := t.fns.SignerService().GetSigner(identity)
	if err != nil {
		return err
	}
	signer := &signerWrapper{creator: identity, signer: s}

	defer t.Close()
	t.proposalResponse, err = t.getProposalResponse(signer)
	if err != nil {
		return err
	}
	return t.appendProposalResponse(t.proposalResponse)
}

func (t *Transaction) AppendProposalResponse(response driver.ProposalResponse) error {
	resp, ok := response.(*ProposalResponse)
	if !ok {
		return fmt.Errorf("wrong proposal response type: %T", response)
	}

	return t.appendProposalResponse(resp.PR())
}

func (t *Transaction) ProposalHasBeenEndorsedBy(party view.Identity) error {
	verifier, err := t.channel.ChannelMembership().GetVerifier(party)
	if err != nil {
		return err
	}
	return verifier.Verify(t.SignedProposal().ProposalBytes(), t.SignedProposal().Signature())
}

func (t *Transaction) StoreTransient() error {
	logger.Debugf("Storing transient for [%s]", t.ID())
	return t.channel.MetadataService().StoreTransient(t.ctx, t.ID(), t.TTransient)
}

func (t *Transaction) ProposalResponses() ([]driver.ProposalResponse, error) {
	res := make([]driver.ProposalResponse, len(t.TProposalResponses))
	for i, resp := range t.TProposalResponses {
		r, err := NewProposalResponseFromResponse(resp)
		if err != nil {
			logger.Errorf("failed creating proposal response from transaction [%s,%v]", t.ID(), err)
			return nil, err
		}
		res[i] = r
	}
	return res, nil
}

func (t *Transaction) ProposalResponse() ([]byte, error) {
	raw, err := proto.Marshal(t.proposalResponse)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (t *Transaction) Envelope() (driver.Envelope, error) {
	env, err := t.createSCEnvelope()
	if err != nil {
		return nil, fmt.Errorf("could not assemble transaction: %w", err)
	}

	res, err := t.Results()
	if err != nil {
		return nil, err
	}

	return NewEnvelope(t.TTxID, t.Nonce(), t.TCreator.Bytes(), res, env), nil
}

func (t *Transaction) generateProposal(signer SerializableSigner) error {
	// Build the spec
	params := append([][]byte{[]byte(t.TFunction)}, t.TParameters...)
	input := pb.ChaincodeInput{
		Args:        params,
		Decorations: nil,
		IsInit:      false,
	}
	spec := &pb.ChaincodeSpec{
		Type:        pb.ChaincodeSpec_GOLANG,
		ChaincodeId: &pb.ChaincodeID{Name: t.TChaincode, Version: t.TChaincodeVersion},
		Input:       &input,
	}
	invocation := &pb.ChaincodeInvocationSpec{ChaincodeSpec: spec}
	funcName := "invoke"
	proposal, _, err := protoutil.CreateChaincodeProposalWithTxIDNonceAndTransient(
		t.TTxID,
		cb.HeaderType_ENDORSER_TRANSACTION,
		t.TChannel, invocation,
		t.TNonce,
		t.TCreator,
		nil)
	if err != nil {
		return fmt.Errorf("error creating proposal for %s", funcName)
	}

	t.TProposal = proposal
	t.TSignedProposal, err = protoutil.GetSignedProposal(proposal, signer)
	if err != nil {
		return fmt.Errorf("error creating underlying signed proposal for %s", funcName)
	}

	t.signedProposal, err = newSignedProposal(t.TSignedProposal)
	if err != nil {
		return err
	}

	logger.Debugf("signed proposal hash set [%x]", t.signedProposal.ProposalHash())
	return nil
}

func (t *Transaction) appendProposalResponse(response *pb.ProposalResponse) error {
	for _, r := range t.TProposalResponses {
		if bytes.Equal(r.Endorsement.Endorser, response.Endorsement.Endorser) {
			logger.Debugf("an endorsement from [%s] found, skip it", view.Identity(r.Endorsement.Endorser))
			return nil
		}
	}

	t.TProposalResponses = append(t.TProposalResponses, response)
	return nil
}

func (t *Transaction) getProposalResponse(signer SerializableSigner) (*pb.ProposalResponse, error) {
	signedProposal := t.SignedProposal()
	if signedProposal == nil {
		return nil, fmt.Errorf("error getting signed proposal [%s]", t.ID())
	}

	logger.Debugf("prepare rws for proposal response [%s]", t.ID())
	rwset, err := t.GetRWSet()
	if err != nil {
		return nil, errors.WithMessagef(err, "error getting rwset for [%s]", t.ID())
	}

	rawTx, err := rwset.Bytes()
	if err != nil {
		return nil, errors.WithMessagef(err, "error serializing rws for [%s]", t.ID())
	}

	var tx protoblocktx.Tx
	if err := proto.Unmarshal(rawTx, &tx); err != nil {
		return nil, fmt.Errorf("failed to deserialize tx [%s]", t.ID())
	}

	// serialize the signing identity
	// sign the concatenation of the proposal response and the serialized endorser identity with this endorser's key
	creator, err := signer.Serialize()
	if err != nil {
		return nil, fmt.Errorf("could not get the signer's identity: %w", err)
	}

	// NOTE: we should go through all namespaces and sign them ... currently a single namespace works
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		jsonTx, _ := json.Marshal(&tx)
		logger.Debugf("endorse tx [%s]", string(jsonTx))
	}

	digest, err := signature.ASN1MarshalTxNamespace(&tx, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot serialize tx: %w", err)
	}

	sig, err := signer.Sign(digest)
	if err != nil {
		return nil, fmt.Errorf("could not sign the proposal response payload: %w", err)
	}

	return &pb.ProposalResponse{
		Version:     1,
		Endorsement: &pb.Endorsement{Signature: sig, Endorser: creator},
		Payload:     rawTx,
		Response: &pb.Response{
			Status:  200,
			Message: "",
			Payload: nil,
		},
	}, nil
}
