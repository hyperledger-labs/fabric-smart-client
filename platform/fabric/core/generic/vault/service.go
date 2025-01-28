/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger("fabric-sdk.core.vault")

func New(configService driver.ConfigService, vaultStore driver3.VaultStore, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) *Vault {
	txIDStoreCacheSize := configService.VaultTXStoreCacheSize()

	if txIDStoreCacheSize > 0 {
		logger.Debugf("creating txID store second cache with size [%d]", txIDStoreCacheSize)
		c := cache.NewCache(vaultStore, secondcache.NewTyped[*cache.Entry](txIDStoreCacheSize), logger)
		return NewVault(c, metricsProvider, tracerProvider)
	} else {
		logger.Debugf("txID store without cache selected")
		c := cache.NewNoCache(vaultStore)
		return NewVault(c, metricsProvider, tracerProvider)
	}
}
