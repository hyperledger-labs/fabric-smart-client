/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// UnmarshalCommand unmarshal Command messages
func UnmarshalCommand(raw []byte) (*protos.Command, error) {
	command := &protos.Command{}
	err := proto.Unmarshal(raw, command)
	if err != nil {
		return nil, err
	}

	return command, nil
}

type TimeFunc func() time.Time

type SignerProvider interface {
	GetSigner(identity view2.Identity) (sig.Signer, error)
}

type Hasher interface {
	Hash(msg []byte) ([]byte, error)
}

// ResponseMarshaler produces SignedCommandResponse
type ResponseMarshaler struct {
	hasher           Hasher
	identityProvider IdentityProvider
	sigService       SignerProvider
	time             TimeFunc
}

func NewResponseMarshaler(hasher Hasher, identityProvider IdentityProvider, sigService SignerProvider) (*ResponseMarshaler, error) {
	return &ResponseMarshaler{
		hasher:           hasher,
		identityProvider: identityProvider,
		sigService:       sigService,
		time:             time.Now,
	}, nil
}

func (s *ResponseMarshaler) MarshalCommandResponse(command []byte, responsePayload interface{}) (*protos.SignedCommandResponse, error) {
	cr, err := commandResponseFromPayload(responsePayload)
	if err != nil {
		return nil, err
	}

	ts := timestamppb.New(s.time())
	if err := ts.CheckValid(); err != nil {
		return nil, err
	}

	did := s.identityProvider.DefaultIdentity()
	cr.Header = &protos.CommandResponseHeader{
		Creator:     did,
		CommandHash: s.computeHash(command),
		Timestamp:   ts,
	}

	return s.createSignedCommandResponse(cr)
}

func (s *ResponseMarshaler) createSignedCommandResponse(cr *protos.CommandResponse) (*protos.SignedCommandResponse, error) {
	raw, err := proto.Marshal(cr)
	if err != nil {
		return nil, err
	}

	did := s.identityProvider.DefaultIdentity()
	dSigner, err := s.sigService.GetSigner(did)
	if err != nil {
		return nil, err
	}
	signature, err := dSigner.Sign(raw)
	if err != nil {
		return nil, err
	}

	return &protos.SignedCommandResponse{
		Response:  raw,
		Signature: signature,
	}, nil
}

func (s *ResponseMarshaler) computeHash(data []byte) (hash []byte) {
	hash, err := s.hasher.Hash(data)
	if err != nil {
		panic(errors.Errorf("failed computing hash on [% x]", data))
	}
	return
}

func commandResponseFromPayload(payload interface{}) (*protos.CommandResponse, error) {
	switch t := payload.(type) {
	case *protos.CommandResponse_Err:
		return &protos.CommandResponse{Payload: t}, nil
	case *protos.CommandResponse_InitiateViewResponse:
		return &protos.CommandResponse{Payload: t}, nil
	case *protos.CommandResponse_CallViewResponse:
		return &protos.CommandResponse{Payload: t}, nil
	default:
		return nil, errors.Errorf("command type not recognized: %T", t)
	}
}
