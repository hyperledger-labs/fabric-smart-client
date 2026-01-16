/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

// TODO: remove this and merge with that in transaction
type UnpackedEnvelope struct {
	NetworkID         string
	TxID              string
	Ch                string
	ChaincodeName     string
	ChaincodeVersion  string
	Input             *peer.ChaincodeInput
	Nonce             []byte
	Creator           []byte
	Results           []byte
	Function          string
	Args              []string
	ChannelHeader     *common.ChannelHeader
	SignatureHeader   *common.SignatureHeader
	ProposalResponses []*peer.ProposalResponse
}

func UnpackEnvelopeFromBytes(networkID string, raw []byte) (*UnpackedEnvelope, error) {
	env := &common.Envelope{}
	if err := proto.Unmarshal(raw, env); err != nil {
		return nil, err
	}
	return UnpackEnvelope(networkID, env)
}

func UnpackEnvelope(networkID string, env *common.Envelope) (*UnpackedEnvelope, error) {
	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetPayload failed, err %s", err)
		return nil, err
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return nil, err
	}

	return UnpackEnvelopeFromPayloadAndCHHeader(networkID, payl, chdr)
}

func UnpackEnvelopeFromPayloadAndCHHeader(networkID string, payl *common.Payload, chdr *common.ChannelHeader) (*UnpackedEnvelope, error) {
	// validate the payload type
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Errorf("Only EndorserClient Transactions are supported, provided type %d", chdr.Type)
		return nil, errors.Errorf("only EndorserClient Transactions are supported, provided type %d", chdr.Type)
	}

	sdr, err := protoutil.UnmarshalSignatureHeader(payl.Header.SignatureHeader)
	if err != nil {
		return nil, err
	}

	// ...and the transaction...
	tx, err := protoutil.UnmarshalTransaction(payl.Data)
	if err != nil {
		logger.Errorf("VSCC error: GetTransaction failed, err %s", err)
		return nil, err
	}

	cap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeActionPayload failed, err %s", err)
		return nil, err
	}
	cpp, err := protoutil.UnmarshalChaincodeProposalPayload(cap.ChaincodeProposalPayload)
	if err != nil {
		logger.Errorf("VSCC error: GetChaincodeProposalPayload failed, err %s", err)
		return nil, err
	}
	cis, err := protoutil.UnmarshalChaincodeInvocationSpec(cpp.Input)
	if err != nil {
		logger.Errorf("VSCC error: UnmarshalChaincodeInvocationSpec failed, err %s", err)
		return nil, err
	}

	pRespPayload, err := protoutil.UnmarshalProposalResponsePayload(cap.Action.ProposalResponsePayload)
	if err != nil {
		err = errors.Errorf("GetProposalResponsePayload error %s", err)
		return nil, err
	}
	if pRespPayload.Extension == nil {
		err = errors.Errorf("nil pRespPayload.Extension")
		return nil, err
	}
	respPayload, err := protoutil.UnmarshalChaincodeAction(pRespPayload.Extension)
	if err != nil {
		err = errors.Errorf("GetChaincodeAction error %s", err)
		return nil, err
	}

	var args []string
	for i := 1; i < len(cis.ChaincodeSpec.Input.Args); i++ {
		args = append(args, string(cis.ChaincodeSpec.Input.Args[i]))
	}

	var proposalResponses []*peer.ProposalResponse
	for _, endorsement := range cap.Action.Endorsements {
		proposalResponses = append(proposalResponses,
			&peer.ProposalResponse{
				Payload:     cap.Action.ProposalResponsePayload,
				Endorsement: endorsement,
			})
	}

	return &UnpackedEnvelope{
		NetworkID:         networkID,
		TxID:              chdr.TxId,
		Ch:                chdr.ChannelId,
		ChaincodeName:     cis.ChaincodeSpec.ChaincodeId.Name,
		ChaincodeVersion:  cis.ChaincodeSpec.ChaincodeId.Version,
		Input:             cis.ChaincodeSpec.Input,
		Nonce:             sdr.Nonce,
		Creator:           sdr.Creator,
		Results:           respPayload.Results,
		Function:          string(cis.ChaincodeSpec.Input.Args[0]),
		Args:              args,
		ChannelHeader:     chdr,
		SignatureHeader:   sdr,
		ProposalResponses: proposalResponses,
	}, nil
}

func (u *UnpackedEnvelope) ID() string {
	return u.TxID
}

func (u *UnpackedEnvelope) Network() string {
	return u.NetworkID
}

func (u *UnpackedEnvelope) Channel() string {
	return u.Ch
}

func (u *UnpackedEnvelope) FunctionAndParameters() (string, []string) {
	return u.Function, u.Args
}
