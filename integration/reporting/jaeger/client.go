/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package jaeger

import (
	"context"
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring/otlp"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultSearchDepth = 20

type Reporter interface {
	FindTraces(nodeName, operationName string) (iterators.Iterator[*api_v2.SpansResponseChunk], error)
	GetServices() ([]string, error)
	GetOperations(nodeName string) ([]string, error)
}

func NewLocalReporter() (*reporter, error) {
	return NewReporter(fmt.Sprintf("0.0.0.0:%d", otlp.JaegerQueryPort))
}

func NewReporter(address string) (*reporter, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &reporter{ClientConn: conn}, nil
}

type reporter struct {
	*grpc.ClientConn
}

func (c *reporter) GetServices() ([]string, error) {
	resp, err := c.queryService().GetServices(context.Background(), &api_v2.GetServicesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetServices(), nil
}

func (c *reporter) GetOperations(nodeName string) ([]string, error) {
	req := &api_v2.GetOperationsRequest{}
	if len(nodeName) > 0 {
		req.Service = nodeName
	}
	resp, err := c.queryService().GetOperations(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.GetOperationNames(), nil
}

func (c *reporter) FindTraces(nodeName, operationName string) (iterators.Iterator[*api_v2.SpansResponseChunk], error) {
	if len(nodeName) == 0 {
		return nil, errors.New("no node name passed")
	}
	params := &api_v2.TraceQueryParameters{ServiceName: nodeName, RawTraces: true, SearchDepth: defaultSearchDepth}
	if len(operationName) > 0 {
		params.OperationName = operationName
	}
	findTraces, err := c.queryService().FindTraces(context.Background(), &api_v2.FindTracesRequest{Query: params})
	if err != nil {
		return nil, err
	}
	return iterators.Stream[*api_v2.SpansResponseChunk](findTraces), nil
}

func (c *reporter) queryService() api_v2.QueryServiceClient {
	return api_v2.NewQueryServiceClient(c.ClientConn)
}
