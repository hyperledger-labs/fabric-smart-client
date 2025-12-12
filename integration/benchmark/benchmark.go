/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package benchmark

import (
	"testing"
)

func ReportTPS(b *testing.B) {
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}
