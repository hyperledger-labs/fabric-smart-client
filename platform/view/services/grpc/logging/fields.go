/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type protoMarshaler struct {
	proto.Message
}

func (m *protoMarshaler) MarshalJSON() ([]byte, error) { return json.Marshal(m.Message) }

func ProtoMessage(key string, val interface{}) zapcore.Field {
	if pm, ok := val.(proto.Message); ok {
		return zap.Reflect(key, &protoMarshaler{pm})
	}
	return zap.Any(key, val)
}

func Error(err error) zapcore.Field {
	if err == nil {
		return zap.Skip()
	}

	// Wrap the error so it no longer implements fmt.Formatter. This will prevent
	// zap from adding the "verboseError" field to the log record that includes a
	// full stack trace.
	return zap.Error(struct{ error }{err})
}
