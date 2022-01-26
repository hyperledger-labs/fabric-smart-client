/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

type Server interface {
	RegisterProcessor(typ reflect.Type, p view2.Processor)
}

type NetworkProvider interface {
	FabricNetworkService(network string) (driver.FabricNetworkService, error)
}

type finalityHandler struct {
	networkProvider NetworkProvider
}

func InstallHandler(server Server, networkProvider NetworkProvider) {
	fh := &finalityHandler{networkProvider: networkProvider}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_IsTxFinal{}), fh.isTxFinal)
}

func (s *finalityHandler) isTxFinal(ctx context.Context, command *protos2.Command) (interface{}, error) {
	c := command.Payload.(*protos2.Command_IsTxFinal).IsTxFinal

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]?", c.Txid, c.Network, c.Channel)
	}

	network, err := s.networkProvider.FabricNetworkService(c.Network)
	if err != nil {
		return nil, errors.Errorf("failed getting network [%s], err [%s]", c.Network, err)
	}
	ch, err := network.Channel(c.Channel)
	if err != nil {
		return nil, errors.Errorf("failed getting finality service for channel [%s], err [%s]", c.Channel, err)
	}
	err = ch.IsFinal(c.Txid)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Answering: Is [%s] final on [%s:%s]? No", c.Txid, c.Network, c.Channel)
		}
		return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{
			Payload: []byte(err.Error()),
		}}, nil
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]? Yes", c.Txid, c.Network, c.Channel)
	}
	return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{}}, nil
}
