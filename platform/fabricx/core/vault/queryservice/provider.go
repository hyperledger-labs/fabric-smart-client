/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice

import (
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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
		return nil, errors.Wrap(err, "get config")
	}

	conn, err := GrpcClient(c)
	if err != nil {
		return nil, errors.Wrap(err, "get grpc client for query service")
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
		return nil, errors.Wrapf(err, "get mapping provider for [channel=%s]", channel)
	}

	return NewRemoteQueryServiceFromConfig(configService)
}

func GetQueryService(sp services.Provider, network, channel string) (QueryService, error) {
	qsp, err := sp.GetService(reflect.TypeOf((*Provider)(nil)))
	if err != nil {
		return nil, errors.Wrap(err, "could not find provider")
	}
	return qsp.(Provider).Get(network, channel)
}
