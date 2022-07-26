/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/base64"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	pcommon "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

type Proposal struct {
	p *pb.Proposal
}

func (p *Proposal) Header() []byte {
	return p.p.Header
}

func (p *Proposal) Payload() []byte {
	return p.p.Payload
}

type SignedProposal struct {
	s  *pb.SignedProposal
	up *UnpackedProposal
}

func newSignedProposal(s *pb.SignedProposal) (*SignedProposal, error) {
	logger.Debugf("new signed proposal with...")
	up, err := UnpackSignedProposal(s)
	if err != nil {
		return nil, err
	}
	logger.Debugf("new signed proposal with [%v]", up.ProposalHash)
	return &SignedProposal{
		s:  s,
		up: up,
	}, nil
}

func (p *SignedProposal) ProposalBytes() []byte {
	return p.s.ProposalBytes
}

func (p *SignedProposal) Signature() []byte {
	return p.s.Signature
}

func (p *SignedProposal) ProposalHash() []byte {
	return p.up.ProposalHash
}

func (p *SignedProposal) ChaincodeName() string {
	return p.up.ChaincodeName
}

func (p *SignedProposal) ChaincodeVersion() string {
	return p.up.ChaincodeVersion
}

type Transaction struct {
	sp               view2.ServiceProvider
	fns              driver.FabricNetworkService
	rwset            driver.RWSet
	channel          driver.Channel
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
	var params []string
	for _, parameter := range t.Parameters() {
		params = append(params, string(parameter))
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
	upr, err := UnpackProposalResponse(t.TProposalResponses[0])
	if err != nil {
		return nil, err
	}
	return upr.Results(), nil
}

func (t *Transaction) From(tx driver.Transaction) (err error) {
	payload := tx.(*Transaction)

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
	err := json.Unmarshal(raw, t)
	if err != nil {
		return errors.Wrapf(err, "SetFromBytes: failed unmarshalling payload [%s]", string(raw))
	}
	logger.Debugf("set transient [%v]", t.TTransient)

	if t.TSignedProposal != nil {
		// TODO: check the current payload is compatible with the content of the signed proposal
		up, err := UnpackSignedProposal(t.TSignedProposal)
		if err != nil {
			return errors.Wrapf(err, "SetFromBytes: failed unpacking proposal [%s]", string(raw))
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
	upe, err := UnpackEnvelopeFromBytes(raw)
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

func (t *Transaction) SetProposal(chaincode string, version string, function string, params ...string) {
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
		return errors.Errorf("invalid index, got [%d]>=[%d]", i, len(t.TParameters))
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
	switch {
	case len(t.TProposalResponses) != 0:
		logger.Debugf("populate rws from proposal response")
		results, err := t.Results()
		if err != nil {
			return err
		}
		t.rwset, err = t.channel.GetRWSet(t.ID(), results)
		if err != nil {
			return err
		}
	case len(t.RWSet) != 0:
		logger.Debugf("populate rws from rwset")
		var err error
		t.rwset, err = t.channel.GetRWSet(t.ID(), t.RWSet)
		if err != nil {
			return err
		}
	default:
		logger.Debugf("populate rws from scratch")
		var err error
		t.rwset, err = t.channel.NewRWSet(t.ID())
		if err != nil {
			return err
		}
	}
	logger.Debugf("rws set [%s]", t.rwset.String())
	return nil
}

func (t *Transaction) RWS() driver.RWSet {
	return t.rwset
}

func (t *Transaction) Done() error {
	if t.rwset != nil {
		// There is a simulation in progress:
		// 1. terminate it
		// 2. append it to the payload
		t.rwset.Done()
		var err error
		t.RWSet, err = t.rwset.Bytes()
		if err != nil {
			return errors.Wrapf(err, "failed marshalling rws")
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("terminated simulation with [%s][len:%d]", t.rwset.Namespaces(), len(t.RWSet))
		}
	}
	return nil
}

func (t *Transaction) Close() {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("closing transaction [%s,%v]", t.ID(), t.rwset != nil)
	}
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
	logger.Debugf("get transient [%v]", t.TTransient)
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
	// prepare signer
	s, err := t.fns.SignerService().GetSigner(identity)
	if err != nil {
		return err
	}
	signer := &signerWrapper{creator: identity, signer: s}

	// is there a proposal already signed?
	if t.SignedProposal() == nil {
		// Endorse it
		if err := t.generateProposal(signer); err != nil {
			return errors.Wrapf(err, "failed setting proposal")
		}
	} else {
		logger.Debugf("signed proposal already set [%v]", t.signedProposal)
	}

	// is there already a response or a simulation is in progress?
	if len(t.TProposalResponses) != 0 || len(t.RWSet) != 0 || t.RWS() != nil {
		defer t.Close()

		t.proposalResponse, err = t.getProposalResponse(signer)
		if err != nil {
			return errors.Wrapf(err, "failed getting proposal response")
		}
		err = t.appendProposalResponse(t.proposalResponse)
		if err != nil {
			return errors.Wrapf(err, "failed appending proposal response")
		}
	}

	err = t.StoreTransient()
	if err != nil {
		return errors.Wrapf(err, "failed storing transient")
	}

	return nil
}

func (t *Transaction) EndorseWithSigner(identity view.Identity, s driver.Signer) error {
	// prepare signer
	var err error
	signer := &signerWrapper{creator: identity, signer: s}

	// is there a proposal already signed?
	if t.SignedProposal() == nil {
		// Endorse it
		if err := t.generateProposal(signer); err != nil {
			return errors.Wrapf(err, "failed setting proposal")
		}
	}

	// is there already a response or a simulation is in progress?
	if len(t.TProposalResponses) != 0 || len(t.RWSet) != 0 || t.RWS() != nil {
		defer t.Close()

		t.proposalResponse, err = t.getProposalResponse(signer)
		if err != nil {
			return errors.Wrapf(err, "failed getting proposal response")
		}
		err = t.appendProposalResponse(t.proposalResponse)
		if err != nil {
			return errors.Wrapf(err, "failed appending proposal response")
		}
	}

	err = t.StoreTransient()
	if err != nil {
		return errors.Wrapf(err, "failed storing transient")
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
	// prepare signer
	s, err := t.fns.SignerService().GetSigner(identity)
	if err != nil {
		return err
	}
	signer := &signerWrapper{creator: identity, signer: s}

	defer t.Close()
	t.proposalResponse, err = t.getProposalResponse(signer)
	if err != nil {
		return nil
	}
	return t.appendProposalResponse(t.proposalResponse)
}

func (t *Transaction) AppendProposalResponse(response driver.ProposalResponse) error {
	return t.appendProposalResponse(response.(*ProposalResponse).pr)
}

func (t *Transaction) ProposalHasBeenEndorsedBy(party view.Identity) error {
	verifier, err := t.channel.GetVerifier(party)
	if err != nil {
		return err
	}
	return verifier.Verify(t.SignedProposal().ProposalBytes(), t.SignedProposal().Signature())
}

func (t *Transaction) StoreTransient() error {
	logger.Debugf("Storing transient for [%s]", t.ID())
	return t.channel.MetadataService().StoreTransient(t.ID(), t.TTransient)
}

func (t *Transaction) ProposalResponses() []driver.ProposalResponse {
	var res []driver.ProposalResponse
	for _, resp := range t.TProposalResponses {
		r, err := NewProposalResponseFromResponse(resp)
		if err != nil {
			panic(err)
		}
		res = append(res, r)
	}
	return res
}

func (t *Transaction) ProposalResponse() ([]byte, error) {
	raw, err := proto.Marshal(t.proposalResponse)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (t *Transaction) generateProposal(signer SerializableSigner) error {
	logger.Debugf("generate proposal...")
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
		pcommon.HeaderType_ENDORSER_TRANSACTION,
		t.TChannel, invocation,
		t.TNonce,
		t.TCreator,
		nil)
	if err != nil {
		return errors.WithMessagef(err, "error creating proposal for %s", funcName)
	}

	t.TProposal = proposal
	t.TSignedProposal, err = protoutil.GetSignedProposal(proposal, signer)
	if err != nil {
		return errors.WithMessagef(err, "error creating underlying signed proposal for %s", funcName)
	}
	t.signedProposal, err = newSignedProposal(t.TSignedProposal)
	if err != nil {
		return err
	}
	logger.Debugf("signed proposal set [%s]", t.signedProposal.ProposalHash())
	return nil
}

func (t *Transaction) appendProposalResponse(response *pb.ProposalResponse) error {
	t.TProposalResponses = append(t.TProposalResponses, response)

	return nil
}

func (t *Transaction) getProposalResponse(signer SerializableSigner) (*pb.ProposalResponse, error) {
	rwset, err := t.GetRWSet()
	if err != nil {
		return nil, err
	}
	pubSimResBytes, err := rwset.Bytes()
	if err != nil {
		return nil, err
	}

	up := t.SignedProposal()
	if up == nil {
		panic("signed proposal must not be nil")
	}
	response := &pb.Response{
		Status:  200,
		Message: "",
		Payload: nil,
	}
	prpBytes, err := protoutil.GetBytesProposalResponsePayload(up.ProposalHash(), response, pubSimResBytes, nil, &pb.ChaincodeID{
		Name:    up.ChaincodeName(),
		Version: up.ChaincodeVersion(),
	})
	logger.Debugf("ProposalResponse [%s][%s]->[%s] \n",
		base64.StdEncoding.EncodeToString(up.ProposalHash()),
		base64.StdEncoding.EncodeToString(pubSimResBytes),
		base64.StdEncoding.EncodeToString(prpBytes),
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create the proposal response")
	}
	// Note, mPrpBytes is the same as prpBytes by default endorsement plugin, but others could change it.
	// serialize the signing identity
	// sign the concatenation of the proposal response and the serialized endorser identity with this endorser's key
	creator, err := signer.Serialize()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get the signer's identity")
	}

	signature, err := signer.Sign(append(prpBytes, creator...))
	if err != nil {
		return nil, errors.Wrapf(err, "could not sign the proposal response payload")
	}
	endorsement := &pb.Endorsement{Signature: signature, Endorser: creator}

	return &pb.ProposalResponse{
		Version:     1,
		Endorsement: endorsement,
		Payload:     prpBytes,
		Response:    response,
	}, nil
}

type Signer interface {
	Sign(message []byte) ([]byte, error)
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)
	Serialize() ([]byte, error)
}

type signerWrapper struct {
	creator view.Identity
	signer  Signer
}

func (s *signerWrapper) Sign(message []byte) ([]byte, error) {
	return s.signer.Sign(message)
}

func (s *signerWrapper) Serialize() ([]byte, error) {
	return s.creator, nil
}
