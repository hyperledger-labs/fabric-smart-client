/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"reflect"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

var metricsType = reflect.TypeOf((*AppMetrics)(nil))

type AppMetrics interface {
	IsEnabled() bool
	SetTracerProvider(*trace.TracerProvider)
}
type Metrics struct {
	IsEnabled     bool
	RequestTracer AppTracer
}

func NewMetrics(enabled bool) *Metrics {
	return &Metrics{IsEnabled: enabled}
}

func (m *Metrics) LaunchOptl(url string) {
	m.SetTracerProvider(NewTraceProvider(NewJaegerExporter(url)))
}

func (m *Metrics) SetTracerProvider(tp trace.TracerProvider) {
	m.RequestTracer = NewLatencyTracer(tp, LatencyTracerOpts{Name: ""})
}

type AppTracer interface {
	Start(string)
	StartAt(string, time.Time)
	AddEvent(string, string)
	AddEventAt(string, string, time.Time)
	End(string, ...string)
	EndAt(string, time.Time, ...string)
	Collectors() []prometheus.Collector
}

func Get(sp view.ServiceProvider) AppMetrics {
	s, err := sp.GetService(metricsType)
	if err != nil {
		panic(err)
	}
	return s.(AppMetrics)

}
