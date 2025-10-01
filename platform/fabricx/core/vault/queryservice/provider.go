/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"fmt"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger/fabric-x-committer/api/protoqueryservice"
)

type QueryService interface {
	GetState(ns driver.Namespace, key driver.PKey) (*driver.VaultValue, error)
	GetStates(map[driver.Namespace][]driver.PKey) (map[driver.Namespace]map[driver.PKey]driver.VaultValue, error)
}

func NewRemoteQueryServiceFromConfig(configService fdriver.ConfigService) (*RemoteQueryService, error) {
	c, err := NewConfig(configService)
	if err != nil {
		return nil, fmt.Errorf("cannot get config for query service: %w", err)
	}

	conn, err := GrpcClient(c)
	if err != nil {
		return nil, fmt.Errorf("cannot get grpc client for query service: %w", err)
	}

	return NewRemoteQueryService(c, protoqueryservice.NewQueryServiceClient(conn)), nil
}

type Provider interface {
	Get(network, channel string) (QueryService, error)
}

func NewProvider(configProvider config.Provider) Provider {
	return &RemoteQueryServiceProvider{
		ConfigProvider: configProvider,
	}
}

type RemoteQueryServiceProvider struct {
	ConfigProvider config.Provider
}

func (r *RemoteQueryServiceProvider) Get(network, channel string) (QueryService, error) {
	configService, err := r.ConfigProvider.GetConfig(network)
	if err != nil {
		return nil, fmt.Errorf("could not get mapping provider for %s: %w", channel, err)
	}

	return NewRemoteQueryServiceFromConfig(configService)
}

func GetQueryService(sp services.Provider, network, channel string) (QueryService, error) {
	qsp, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		return nil, fmt.Errorf("could not find provider: %w", err)
	}
	return qsp.(Provider).Get(network, channel)
}
