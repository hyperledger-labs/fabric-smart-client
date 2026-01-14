/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/storage/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

func New(
	configService fdriver.ConfigService,
	vaultStore driver.VaultStore,
	channel string,
	_ queryservice.Provider,
	metricsProvider metrics.Provider,
	tracerProvider trace.TracerProvider,
) (*Vault, error) {
	// TODO: this is an example how to integrate the query service into the vault and let all getters communicate with the committer directly
	// queryService, err := queryServiceProvider.Get(configService.NetworkName(), channel)
	// if err != nil {
	//	 return nil, nil, fmt.Errorf("could not get mapping provider for %s: %v", channel, err)
	// }
	//
	// wrapp our store with our remote proxy
	// s := queryservice.NewProxyStore(persistence, queryService)

	cachedVault := vault.NewCachedVault(vaultStore, configService.VaultTXStoreCacheSize())
	return NewVault(cachedVault, metricsProvider, tracerProvider), nil
}
