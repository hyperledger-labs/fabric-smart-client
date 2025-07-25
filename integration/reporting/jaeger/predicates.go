/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jaeger

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

var (
	IsTransfer    = ContainsSpanWithName("/idap/assets/currencies/transfers")
	IsWithdrawal  = ContainsSpanWithName("/idap/assets/currencies/withdrawals")
	IsTransaction = iterators.Or(IsTransfer, IsWithdrawal)
)

func ContainsSpanWithName(name string) func(t *api_v2.SpansResponseChunk) bool {
	return func(t *api_v2.SpansResponseChunk) bool {
		for _, s := range t.Spans {
			if s.OperationName == name {
				return true
			}
		}
		return false
	}
}
