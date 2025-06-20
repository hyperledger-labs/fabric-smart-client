/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

type Server interface {
	RegisterProcessor(typ reflect.Type, p view2.Processor)
}

type Handler interface {
	IsFinal(ctx context.Context, network, channel, txID string) error
}

func NewManager(tracerProvider trace.TracerProvider) *Manager {
	return &Manager{tracer: tracerProvider.Tracer("finality_manager", tracing.WithMetricsOpts(tracing.MetricsOpts{
		Namespace:  "viewsdk",
		LabelNames: []tracing.LabelName{},
	}))}
}

type Manager struct {
	Handlers []Handler
	tracer   trace.Tracer
}

func (s *Manager) AddHandler(handler Handler) {
	s.Handlers = append(s.Handlers, handler)
}
