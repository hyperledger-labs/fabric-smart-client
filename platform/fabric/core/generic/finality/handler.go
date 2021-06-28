/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
)

type Server interface {
	RegisterProcessor(typ reflect.Type, p server.Processor)
}

type finalityHandler struct {
	network Network
}

func InstallHandler(server Server, network Network) {
	fh := &finalityHandler{network: network}
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_IsTxFinal{}), fh.isTxFinal)
}

func (s *finalityHandler) isTxFinal(ctx context.Context, command *protos.Command) (interface{}, error) {
	isTxFinalCommand := command.Payload.(*protos.Command_IsTxFinal).IsTxFinal

	logger.Debugf("Answering: Is [%s] final?", isTxFinalCommand.Txid)

	ch, err := s.network.Channel(isTxFinalCommand.Channel)
	if err != nil {
		return nil, errors.Errorf("failed getting finality service for channel [%s], err [%s]", isTxFinalCommand.Channel, err)
	}

	err = ch.IsFinal(isTxFinalCommand.Txid)
	if err != nil {
		logger.Debugf("Answering: Is [%s] final? No", isTxFinalCommand.Txid)
		return &protos.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos.IsTxFinalResponse{
			Payload: []byte(err.Error()),
		}}, nil
	}

	logger.Debugf("Answering: Is [%s] final? Yes", isTxFinalCommand.Txid)
	return &protos.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos.IsTxFinalResponse{}}, nil
}
