/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

// A Marshaler is responsible for marshaling and signing command responses.
type Marshaler interface {
	MarshalCommandResponse(command []byte, responsePayload interface{}) (*protos2.SignedCommandResponse, error)
}

type Processor func(ctx context.Context, command *protos2.Command) (interface{}, error)

type Streamer func(sc *protos2.SignedCommand, command *protos2.Command, commandServer protos2.ViewService_StreamCommandServer, marshaler Marshaler) error

type Service interface {
	protos2.ViewServiceServer

	RegisterProcessor(typ reflect.Type, p Processor)
	RegisterStreamer(typ reflect.Type, streamer Streamer)
}

func GetService(sp view.ServiceProvider) Service {
	s, err := sp.GetService(reflect.TypeOf((*Service)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(Service)
}
