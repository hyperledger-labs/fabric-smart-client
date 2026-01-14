/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package core

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

type Provider struct {
	p *driver2.Provider
}

func NewProvider(
	configProvider config.Provider,
	metricsProvider metrics.Provider,
	endpointService identity.EndpointService,
	channelProvider generic.ChannelProvider,
	idProvider identity.ViewIdentityProvider,
	identityLoaders []identity.NamedIdentityLoader,
	kvss *kvs.KVS,
	signerKVS driver.SignerInfoStore,
	auditInfoKVS driver.AuditInfoStore,
) *Provider {
	return &Provider{
		p: driver2.NewProvider(
			configProvider,
			metricsProvider,
			endpointService,
			channelProvider,
			idProvider,
			identityLoaders,
			signerKVS,
			auditInfoKVS,
			kvss,
		),
	}
}

func (d *Provider) New(network string, b bool) (fdriver.FabricNetworkService, error) {
	net, err := d.p.New(network, b)
	if err != nil {
		return nil, err
	}

	txManager := transaction.NewManager()
	txManager.AddTransactionFactory(
		fdriver.EndorserTransaction,
		transaction.NewFactory(net),
	)

	net.(*generic.Network).SetTransactionManager(txManager)

	return net, nil
}
