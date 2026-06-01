/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

type OperationsLogger interface {
	Logger
}

type operationsLogger struct {
	Logger
}

func NewOperationsLogger(l Logger) *operationsLogger {
	if l == nil {
		l = logging.MustGetLogger()
	}
	return &operationsLogger{Logger: l}
}
