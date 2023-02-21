/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"go.opentelemetry.io/otel/trace"
)

var metricsType = reflect.TypeOf((*AppMetrics)(nil))

type AppTracer interface {
	Start(string)
	StartAt(string, time.Time)
	AddEvent(string, string)
	AddEventAt(string, string, time.Time)
	End(string, ...string)
	EndAt(string, time.Time, ...string)
}

type AppMetrics interface {
	IsEnabled() bool
	GetTracer() AppTracer
	LaunchOptl(url string)
}
type Metrics struct {
	enabled       bool
	requestTracer AppTracer
}

func NewMetrics(enabled bool) AppMetrics {
	return &Metrics{enabled: enabled}
}

func (m *Metrics) LaunchOptl(url string) {
	tp := NewTraceProvider(NewJaegerExporter(url))
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
		panic(err)
	}
	return s.(AppMetrics)

}
