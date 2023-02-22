/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"context"
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var logger = flogging.MustGetLogger("tracing")
var metricsType = reflect.TypeOf((*AppMetrics)(nil))

type AppTracer interface {
	Start(string)
	StartAt(string, time.Time)
	AddEvent(string, string)
	AddEventAt(string, string, time.Time)
	End(string, ...string)
	EndAt(string, time.Time, ...string)
	AddError(key string, err error)
}

type AppMetrics interface {
	IsEnabled() bool
	GetTracer() AppTracer
	LaunchOptl(url string, context context.Context)
}
type Metrics struct {
	enabled       bool
	requestTracer AppTracer
}

func NewMetrics(enabled bool) AppMetrics {
	return &Metrics{enabled: enabled}
}

func (m *Metrics) LaunchOptl(url string, context context.Context) {
	r := newResource(context)
	tp := NewTraceProvider(NewHTTPExporter(url, context), r)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetTracerProvider(tp)
	m.requestTracer = setTracerProvider(m, tp)
}

func (m *Metrics) GetTracer() AppTracer {
	return m.requestTracer
}
func (m *Metrics) IsEnabled() bool {
	return m.enabled
}

func setTracerProvider(m *Metrics, tp trace.TracerProvider) AppTracer {
	return NewLatencyTracer(tp, LatencyTracerOpts{Name: "FSC-Tracing"})
}

func Get(sp view.ServiceProvider) AppMetrics {
	s, err := sp.GetService(metricsType)
	if err != nil {
		logger.Panic(err)
	}
	return s.(AppMetrics)

}
