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

func BenchmarkECDSASign(b *testing.B) {
	f := &ECDSASignViewFactory{}
	p := &ECDSASignParams{}
	input, _ := json.Marshal(p)

	b.RunParallel(func(pb *testing.PB) {
		// note that each benchmark goroutine gets their own
		v, _ := f.NewView(input)
		for pb.Next() {
			_, _ = v.Call(nil)
		}
	})
	benchmark.ReportTPS(b)
}

func BenchmarkECDSASign_wFactory(b *testing.B) {
	f := &ECDSASignViewFactory{}
	p := &ECDSASignParams{}
	input, _ := json.Marshal(p)

	b.RunParallel(func(pb *testing.PB) {
		// note that each benchmark goroutine gets their own
		for pb.Next() {
			v, _ := f.NewView(input)
			_, _ = v.Call(nil)
		}
	})
	benchmark.ReportTPS(b)
}
