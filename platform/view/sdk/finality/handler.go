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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

const (
	handlerTypeLabel tracing.LabelName = "handler_type"
	successLabel     tracing.LabelName = "success"
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

func NewManager(tracerProvider trace.TracerProvider) *Manager {
	return &Manager{tracer: tracerProvider.Tracer("finality_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "viewsdk",
		LabelNames: []tracing.LabelName{handlerTypeLabel, successLabel},
	}))}
}

type Manager struct {
	Handlers []Handler
	tracer   trace.Tracer
}

func (s *Manager) IsTxFinal(ctx context.Context, command *protos.Command) (interface{}, error) {
	newCtx, span := s.tracer.Start(ctx, "is_final")
	defer span.End()
	c := command.Payload.(*protos.Command_IsTxFinal).IsTxFinal

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]?", c.Txid, c.Network, c.Channel)
	}

	for _, handler := range s.Handlers {
		span.AddEvent("start_handler", tracing.WithAttributes(tracing.String(handlerTypeLabel, reflect.TypeOf(handler).String())))
		if err := handler.IsFinal(newCtx, c.Network, c.Channel, c.Txid); err == nil {
			span.AddEvent("end_handler", tracing.WithAttributes(tracing.Bool(successLabel, true)))
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Answering: Is [%s] final on [%s:%s]? Yes", c.Txid, c.Network, c.Channel)
			}
			return &protos.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos.IsTxFinalResponse{}}, nil
		} else {
			span.AddEvent("end_handler", tracing.WithAttributes(tracing.Bool(successLabel, false)))
			logger.Debugf("Answering: Is [%s] final on [%s:%s]? err [%s]", c.Txid, c.Network, c.Channel, err)
		}
	}

	return &protos.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos.IsTxFinalResponse{
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
