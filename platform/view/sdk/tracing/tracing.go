/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
)

// This wrapper is needed in order to be able to fetch the provider using the SP from the Node
var providerType = reflect.TypeOf((*tracerProvider)(nil))

type tracerProvider struct {
	tracing.Provider
	viewTracer trace.Tracer
}

func NewWrappedTracerProvider(tp tracing.Provider) tracing.Provider {
	return &tracerProvider{Provider: tp, viewTracer: tp.Tracer("view_tracer")}
}

func Get(sp services.Provider) tracing.Provider {
	s, err := sp.GetService(providerType)
	if err != nil {
		panic(err)
	}
	return s.(*tracerProvider)
}
