/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	log2 "github.com/go-kit/log"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

type OperationsLogger interface {
	Logger
	log2.Logger
}

type operationsLogger struct {
	Logger
}

func (l *operationsLogger) Log(keyvals ...interface{}) error {
	l.Warn(keyvals...)
	return nil
}

func NewOperationsLogger(l Logger) *operationsLogger {
	if l == nil {
		l = logging.MustGetLogger()
	}
	return &operationsLogger{Logger: l}
}
