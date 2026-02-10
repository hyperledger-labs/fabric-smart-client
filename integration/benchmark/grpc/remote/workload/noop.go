/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package workload

import (
	"context"

	"google.golang.org/grpc"
)

const NOOP = "noop"

func CreateClientFuncNoop(_ *grpc.ClientConn) ClientFunc {
	return func(ctx context.Context) error {
		return nil
	}
}
