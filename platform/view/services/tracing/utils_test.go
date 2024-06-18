/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestUnmarshal(t *testing.T) {
	ctx, err := UnmarshalContext([]byte(`{"trace_id":"00000000000000000000000000000000","span_id":"0000000000000000","trace_flags":0,"trace_state":"","remote":false}`))
	assert.NoError(err)
	assert.True(ctx.Equal(trace.SpanContext{}))
}

func TestMarshalUnmarshal(t *testing.T) {
	state, err := trace.ParseTraceState("abcdefghijklmnopqrstuvwxyz0123456789_-*/= !\"#$%&'()*+-./0123456789:;<>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~")
	assert.NoError(err)
	ctx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		TraceState: state,
		Remote:     true,
	})

	m, err := MarshalContext(ctx)
	assert.NoError(err)

	newCtx, err := UnmarshalContext(m)
	assert.NoError(err)

	assert.True(ctx.Equal(newCtx))
}
