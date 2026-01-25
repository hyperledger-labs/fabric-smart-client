/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/stretchr/testify/require"
)

func BenchmarkView(b *testing.B) {
	srvEndpoint := setupServer(b, "cpu")

	// we share a single connection among all client goroutines
	cli, closeF := setupClient(b, srvEndpoint)
	defer closeF()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.CallViewWithContext(b.Context(), "fid", nil)
			require.NoError(b, err)
			require.NotNil(b, resp)
		}
	})
	benchmark.ReportTPS(b)
}
