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

const ECHO = "echo"

func CreateClientFuncEcho(conn *grpc.ClientConn) ClientFunc {
	var msg = &remote.Request{
		Workload: ECHO,
		Input:    []byte("Hello"),
	}

	client := remote.NewBenchmarkServiceClient(conn)
	return func(ctx context.Context) error {
		// DODO change
		resp, err := client.Process(ctx, msg)
		if err != nil {
			return err
		}
		_ = resp
		return nil
	}
}

func ProcessEcho(_ context.Context, in *remote.Request) (*remote.Response, error) {
	return &remote.Response{Output: in.Input}, nil
}
