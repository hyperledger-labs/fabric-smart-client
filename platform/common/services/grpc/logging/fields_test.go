/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestProtoMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value any
	}{
		{"ProtoMessage", wrapperspb.String("test")},
		{"NonProtoValue", "regular string"},
		{"NilValue", nil},
		{"StructValue", struct{ Field string }{Field: "value"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			core, observed := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			logger.Info("test", ProtoMessage("key", tt.value))

			entries := observed.All()
			require.Len(t, entries, 1)
			require.Len(t, entries[0].Context, 1)
			require.Equal(t, "key", entries[0].Context[0].Key)
		})
	}
}

func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		expectedType zapcore.FieldType
	}{
		{"NilError", nil, zapcore.SkipType},
		{"StandardError", errors.New("test error"), zapcore.ErrorType},
		{"WrappedError", errors.Join(errors.New("err1"), errors.New("err2")), zapcore.ErrorType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			core, observed := observer.New(zapcore.InfoLevel)
			logger := zap.New(core)

			logger.Info("test", Error(tt.err))

			entries := observed.All()
			require.Len(t, entries, 1)
			require.Len(t, entries[0].Context, 1)
			require.Equal(t, tt.expectedType, entries[0].Context[0].Type)
		})
	}
}

func TestProtoMarshaler_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message *wrapperspb.StringValue
	}{
		{"WithValue", wrapperspb.String("test")},
		{"EmptyMessage", &wrapperspb.StringValue{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pm := &protoMarshaler{Message: tt.message}
			jsonBytes, err := pm.MarshalJSON()

			require.NoError(t, err)
			require.NotNil(t, jsonBytes)
		})
	}
}

// Made with Bob
