/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/pkg/errors"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

type Server interface {
	RegisterProcessor(typ reflect.Type, p view2.Processor)
}

type finalityHandler struct {
	network Network
}

func InstallHandler(server Server, network Network) {
	fh := &finalityHandler{network: network}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_IsTxFinal{}), fh.isTxFinal)
}

func (s *finalityHandler) isTxFinal(ctx context.Context, command *protos2.Command) (interface{}, error) {
	isTxFinalCommand := command.Payload.(*protos2.Command_IsTxFinal).IsTxFinal

	logger.Debugf("Answering: Is [%s] final?", isTxFinalCommand.Txid)

	ch, err := s.network.Channel(isTxFinalCommand.Channel)
	if err != nil {
		return nil, errors.Errorf("failed getting finality service for channel [%s], err [%s]", isTxFinalCommand.Channel, err)
	}

	err = ch.IsFinal(isTxFinalCommand.Txid)
	if err != nil {
		logger.Debugf("Answering: Is [%s] final? No", isTxFinalCommand.Txid)
		return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{
			Payload: []byte(err.Error()),
		}}, nil
	}

	logger.Debugf("Answering: Is [%s] final? Yes", isTxFinalCommand.Txid)
	return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{}}, nil
}
