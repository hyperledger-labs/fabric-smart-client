/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"
)

// GRPCClientProvider provides gRPC client connections for a given network.
//
//go:generate counterfeiter -o mock/grpc_client_provider.go --fake-name GRPCClientProvider . GRPCClientProvider
type GRPCClientProvider interface {
	// QueryServiceClient returns a gRPC client connection for the specified network.
	QueryServiceClient(network string) (*grpc.ClientConn, error)
}

type ConfigTransactionInfo struct {
	Envelope *common.Envelope
	Version  uint64
}

//go:generate counterfeiter -o mock/query_service.go --fake-name QueryService . QueryService
type QueryService interface {
	GetState(ns driver.Namespace, key driver.PKey) (*driver.VaultValue, error)
	GetStates(map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error)
	GetTransactionStatus(txID string) (int32, error)
	GetConfigTransaction() (*ConfigTransactionInfo, error)
}

// ServiceConfigProvider provides gRPC configuration for a given network.
//
//go:generate counterfeiter -o mock/service_config_provider.go --fake-name ServiceConfigProvider . ServiceConfigProvider
type ServiceConfigProvider interface {
	// QueryServiceConfig returns the configuration for the query service for the specified network.
	QueryServiceConfig(network string) (*config.Config, error)
}

type Provider interface {
	Get(network, channel string) (QueryService, error)
}

func NewProvider(grpcClientProvider GRPCClientProvider, configProvider ServiceConfigProvider) Provider {
	return &RemoteQueryServiceProvider{
		GRPCClientProvider: grpcClientProvider,
		ConfigProvider:     configProvider,
	}
}

type RemoteQueryServiceProvider struct {
	GRPCClientProvider GRPCClientProvider
	ConfigProvider     ServiceConfigProvider
}

func (r *RemoteQueryServiceProvider) Get(network, channel string) (QueryService, error) {
	cc, err := r.GRPCClientProvider.QueryServiceClient(network)
	if err != nil {
		return nil, errors.Wrapf(err, "get grpc client for query service [network=%s, channel=%s]", network, channel)
	}

	// Get config for the query service
	config, err := r.ConfigProvider.QueryServiceConfig(network)
	if err != nil {
		return nil, errors.Wrapf(err, "get config for [network=%s, channel=%s]", network, channel)
	}

	client := committerpb.NewQueryServiceClient(cc)
	return NewRemoteQueryService(config, client), nil
}

func GetQueryService(sp services.Provider, network, channel string) (QueryService, error) {
	qsp, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		return nil, errors.Wrap(err, "could not find provider")
	}
	return qsp.(Provider).Get(network, channel)
}
