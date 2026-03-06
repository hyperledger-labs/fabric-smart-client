/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// A Marshaller is responsible for marshaling and signing command responses.
type Marshaller interface {
	// MarshalCommandResponse marshals and signs the given response payload.
	MarshalCommandResponse(command []byte, responsePayload interface{}) (*protos.SignedCommandResponse, error)
}

type Processor func(ctx context.Context, command *protos.Command) (interface{}, error)

type Streamer func(sc *protos.SignedCommand, command *protos.Command, commandServer protos.ViewService_StreamCommandServer, marshaler Marshaller) error

// Service models the view service server.
type Service interface {
	protos.ViewServiceServer

	// RegisterProcessor registers a processor for the given command payload type.
	RegisterProcessor(typ reflect.Type, p Processor)
	// RegisterStreamer registers a streamer for the given command payload type.
	RegisterStreamer(typ reflect.Type, streamer Streamer)
}

// GetService returns the view service from the service provider.
// It panics if no service is found.
func GetService(sp services.Provider) Service {
	s, err := sp.GetService(reflect.TypeOf((*Service)(nil)))
	if err != nil {
		panic(err)
	}
	return s.(Service)
}
