/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpclogging

import (
	"context"

	"go.uber.org/zap/zapcore"
)

type fieldKeyType struct{}

var fieldKey = &fieldKeyType{}

func WithFields(ctx context.Context, fields []zapcore.Field) context.Context {
	return context.WithValue(ctx, fieldKey, fields)
}
