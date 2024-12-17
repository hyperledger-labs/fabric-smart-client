/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"

var logger = logging.MustGetLogger("batch-executor")

type BatchExecutor[I any, O any] interface {
	Execute(input I) (O, error)
}

type BatchRunner[V any] interface {
	Run(v V) error
}

type Output[O any] struct {
	Val O
	Err error
}

type ExecuteFunc[I any, O any] func([]I) []O
