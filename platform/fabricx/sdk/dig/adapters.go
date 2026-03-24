/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ordering"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

// queryServiceAdapter adapts queryservice.QueryService to channelconfig.QueryService
type queryServiceAdapter struct {
	qs queryservice.QueryService
}

func (a *queryServiceAdapter) GetConfigTransaction() (*channelconfig.ConfigTransactionInfo, error) {
	info, err := a.qs.GetConfigTransaction()
	if err != nil {
		return nil, err
	}
	return &channelconfig.ConfigTransactionInfo{
		Envelope: info.Envelope,
		Version:  info.Version,
	}, nil
}

// orderingServiceAdapter adapts fdriver.Ordering to channelconfig.OrderingService
type orderingServiceAdapter struct {
	os fdriver.Ordering
}

func (a *orderingServiceAdapter) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	// Cast to *ordering.Service which has the Configure method
	orderingService, ok := a.os.(*ordering.Service)
	if !ok {
		return errors.New("ordering service is not an *ordering.Service")
	}
	return orderingService.Configure(consensusType, orderers)
}

// Made with Bob
