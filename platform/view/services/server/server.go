/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
)

// A Marshaler is responsible for marshaling and signing command responses.
type Marshaler interface {
	MarshalCommandResponse(command []byte, responsePayload interface{}) (*protos.SignedCommandResponse, error)
}

type Processor func(ctx context.Context, command *protos.Command) (interface{}, error)

type Streamer func(sc *protos.SignedCommand, command *protos.Command, commandServer protos.ViewService_StreamCommandServer, marshaler Marshaler) error

type ViewServiceServer interface {
	protos.ViewServiceServer

	RegisterProcessor(typ reflect.Type, p Processor)
	RegisterStreamer(typ reflect.Type, streamer Streamer)
}

func GetServer(sp view.ServiceProvider) ViewServiceServer {
	s, err := sp.GetService(reflect.TypeOf((*ViewServiceServer)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(ViewServiceServer)
}
