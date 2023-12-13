/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"go.uber.org/zap/zapcore"
)

var (
	managerType = reflect.TypeOf((*Manager)(nil))
	logger      = flogging.MustGetLogger("view-sdk.finality")
)

type Registry interface {
	RegisterService(service interface{}) error
}

type Server interface {
	RegisterProcessor(typ reflect.Type, p view2.Processor)
}

type Handler interface {
	IsFinal(ctx context.Context, network, channel, txID string) error
}

type Manager struct {
	Handlers []Handler
}

func InstallHandler(registry Registry, server Server) error {
	fh := &Manager{}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_IsTxFinal{}), fh.isTxFinal)
	return registry.RegisterService(fh)
}

func (s *Manager) isTxFinal(ctx context.Context, command *protos2.Command) (interface{}, error) {
	c := command.Payload.(*protos2.Command_IsTxFinal).IsTxFinal

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]?", c.Txid, c.Network, c.Channel)
	}

	for _, handler := range s.Handlers {
		if err := handler.IsFinal(ctx, c.Network, c.Channel, c.Txid); err == nil {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Answering: Is [%s] final on [%s:%s]? Yes", c.Txid, c.Network, c.Channel)
			}
			return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{}}, nil
		} else {
			logger.Debugf("Answering: Is [%s] final on [%s:%s]? err [%s]", c.Txid, c.Network, c.Channel, err)
		}
	}

	return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{
		Payload: []byte("no handler found for the request"),
	}}, nil
}

func (s *Manager) AddHandler(handler Handler) {
	s.Handlers = append(s.Handlers, handler)
}

func GetManager(sp view.ServiceProvider) *Manager {
	s, err := sp.GetService(managerType)
	if err != nil {
		logger.Warnf("failed getting finality manager: %s", err)
		return nil
	}
	return s.(*Manager)
}
