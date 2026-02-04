/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package workload

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote"
	"google.golang.org/grpc"
)

const CPU = "cpu"

func CreateClientFuncCPU(conn *grpc.ClientConn) ClientFunc {
	var msg = &remote.Request{
		Workload: CPU,
		Input:    []byte("Hello"),
	}

	client := remote.NewBenchmarkServiceClient(conn)
	return func(ctx context.Context) error {
		resp, err := client.Process(ctx, msg)
		if err != nil {
			return err
		}
		_ = resp
		return nil
	}
}

func ProcessCPU(_ context.Context, in *remote.Request) (*remote.Response, error) {
	r := doWork(200000)
	_ = r
	return &remote.Response{Output: in.Input}, nil
}

// doWork just burns CPU cycles
func doWork(n int) uint64 {
	var x uint64
	for i := 0; i < n; i++ {
		x += uint64(i * i)
	}
	return x
}
