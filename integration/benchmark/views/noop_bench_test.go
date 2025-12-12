/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
)

func BenchmarkNoop(b *testing.B) {
	f := &NoopViewFactory{}
	b.RunParallel(func(pb *testing.PB) {
		v, _ := f.NewView(nil)
		for pb.Next() {
			_, _ = v.Call(nil)
		}
	})
	benchmark.ReportTPS(b)
}
