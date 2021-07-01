/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	hash2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

// UnmarshalCommand unmarshal Command messages
func UnmarshalCommand(raw []byte) (*protos2.Command, error) {
	command := &protos2.Command{}
	err := proto.Unmarshal(raw, command)
	if err != nil {
		return nil, err
	}

	return command, nil
}

type TimeFunc func() time.Time

// ResponseMarshaler produces SignedCommandResponse
type ResponseMarshaler struct {
	sp   driver.ServiceProvider
	time TimeFunc
}

func NewResponseMarshaler(sp driver.ServiceProvider) (*ResponseMarshaler, error) {
	return &ResponseMarshaler{
		sp:   sp,
		time: time.Now,
	}, nil
}

func (s *ResponseMarshaler) MarshalCommandResponse(command []byte, responsePayload interface{}) (*protos2.SignedCommandResponse, error) {
	cr, err := commandResponseFromPayload(responsePayload)
	if err != nil {
		return nil, err
	}

	ts, err := ptypes.TimestampProto(s.time())
	if err != nil {
		return nil, err
	}

	did := driver.GetIdentityProvider(s.sp).DefaultIdentity()
	cr.Header = &protos2.CommandResponseHeader{
		Creator:     did,
		CommandHash: s.computeHash(command),
		Timestamp:   ts,
	}

	return s.createSignedCommandResponse(cr)
}

func (s *ResponseMarshaler) createSignedCommandResponse(cr *protos2.CommandResponse) (*protos2.SignedCommandResponse, error) {
	raw, err := proto.Marshal(cr)
	if err != nil {
		return nil, err
	}

	did := driver.GetIdentityProvider(s.sp).DefaultIdentity()
	dSigner, err := view.GetSigService(s.sp).GetSigner(did)
	if err != nil {
		return nil, err
	}
	signature, err := dSigner.Sign(raw)
	if err != nil {
		return nil, err
	}

	return &protos2.SignedCommandResponse{
		Response:  raw,
		Signature: signature,
	}, nil
}

func (s *ResponseMarshaler) computeHash(data []byte) (hash []byte) {
	hash, err := hash2.GetHasher(s.sp).Hash(data)
	if err != nil {
		panic(fmt.Errorf("failed computing hash on [% x]", data))
	}
	return
}

func commandResponseFromPayload(payload interface{}) (*protos2.CommandResponse, error) {
	switch t := payload.(type) {
	case *protos2.CommandResponse_Err:
		return &protos2.CommandResponse{Payload: t}, nil
	case *protos2.CommandResponse_InitiateViewResponse:
		return &protos2.CommandResponse{Payload: t}, nil
	case *protos2.CommandResponse_TrackViewResponse:
		return &protos2.CommandResponse{Payload: t}, nil
	case *protos2.CommandResponse_CallViewResponse:
		return &protos2.CommandResponse{Payload: t}, nil
	case *protos2.CommandResponse_IsTxFinalResponse:
		return &protos2.CommandResponse{Payload: t}, nil
	default:
		return nil, errors.Errorf("command type not recognized: %T", t)
	}
}
