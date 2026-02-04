/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package workload

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote"
)

type ClientFunc func(ctx context.Context) error
type ServerFunc func(ctx context.Context, in *remote.Request) (*remote.Response, error)
