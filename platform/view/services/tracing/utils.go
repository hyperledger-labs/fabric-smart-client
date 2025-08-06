/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"encoding/json"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"go.opentelemetry.io/otel/trace"
)

func MarshalContext(c trace.SpanContext) ([]byte, error) {
	return json.Marshal(&spanContext{
		TraceID:    c.TraceID().String(),
		SpanID:     c.SpanID().String(),
		TraceFlags: byte(c.TraceFlags()),
		TraceState: c.TraceState().String(),
		Remote:     c.IsRemote(),
	})
}

func UnmarshalContext(data []byte) (trace.SpanContext, error) {
	var sc spanContext
	if err := json.Unmarshal(data, &sc); err != nil {
		return trace.SpanContext{}, errors.Wrapf(err, "failed unmarshalling span context from [%s]", string(data))
	}
	if sc.IsEmpty() {
		return trace.SpanContext{}, nil
	}

	traceState, err := trace.ParseTraceState(sc.TraceState)
	if err != nil {
		return trace.SpanContext{}, errors.Wrapf(err, "failed unmarshalling trace state from [%s]", string(sc.TraceState))
	}
	spanID, err := trace.SpanIDFromHex(sc.SpanID)
	if err != nil {
		return trace.SpanContext{}, errors.Wrapf(err, "failed unmarshalling span ID from [%s]", string(sc.SpanID))
	}
	traceID, err := trace.TraceIDFromHex(sc.TraceID)
	if err != nil {
		return trace.SpanContext{}, errors.Wrapf(err, "failed unmarshalling trace ID from [%s]", string(sc.TraceID))
	}

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(sc.TraceFlags),
		TraceState: traceState,
		Remote:     sc.Remote,
	}), nil
}

type spanContext struct {
	TraceID    string `json:"trace_id"`
	SpanID     string `json:"span_id"`
	TraceFlags byte   `json:"trace_flags"`
	TraceState string `json:"trace_state"`
	Remote     bool   `json:"remote"`
}

func (c *spanContext) IsEmpty() bool {
	return len(strings.Trim(c.TraceID, "0")) == 0 &&
		len(strings.Trim(c.SpanID, "0")) == 0 &&
		len(strings.Trim(c.TraceState, "0")) == 0 &&
		c.TraceFlags == 0 &&
		!c.Remote
}
