/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var (
	logger      = flogging.MustGetLogger("tracing")
	metricsType = reflect.TypeOf((*Provider)(nil))
)

type Tracer interface {
	// Start creates a span starting now
	Start(spanName string)
	// StartAt creates a span starting from a given time
	StartAt(spanName string, when time.Time)
	// AddEvent adds an event to an existing span with the provided name.
	AddEvent(spanName string, eventName string)
	// AddEventAt adds an event to an existing span with the provided name and timestamp
	AddEventAt(spanName string, eventName string, when time.Time)
	// AddError adds an error to an existing span
	AddError(spanName string, err error)
	// End completes an existing span.
	End(spanName string, attrs ...string)
	// EndAt completes an existing span with a given timestamp.
	EndAt(spanName string, when time.Time, attrs ...string)
}

type Provider struct {
	tracer Tracer
}

func NewProvider(tracer Tracer) *Provider {
	return &Provider{tracer: tracer}
}

func (m *Provider) GetTracer() Tracer {
	return m.tracer
}

func Get(sp view.ServiceProvider) *Provider {
	s, err := sp.GetService(metricsType)
	if err != nil {
		logger.Panic(err)
	}
	return s.(*Provider)
}
