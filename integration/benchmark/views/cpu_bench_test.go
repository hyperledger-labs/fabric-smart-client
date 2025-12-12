/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
)

func BenchmarkCPU(b *testing.B) {

	f := &CPUViewFactory{}
	// tune up/down for longer/shorter ops
	p := &CPUParams{N: 200000}
	input, _ := json.Marshal(p)

	b.RunParallel(func(pb *testing.PB) {
		v, _ := f.NewView(input)
		for pb.Next() {
			_, _ = v.Call(nil)
		}
	})
	benchmark.ReportTPS(b)
}
