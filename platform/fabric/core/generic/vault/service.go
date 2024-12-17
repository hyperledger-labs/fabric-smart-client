/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger("fabric-sdk.core.vault")

func New(configService driver.ConfigService, channel string, drivers []driver2.NamedDriver, metricsProvider metrics.Provider, tracerProvider trace.TracerProvider) (*Vault, driver.TXIDStore, error) {
	var d driver2.Driver
	for _, driver := range drivers {
		if driver.Name == configService.VaultPersistenceType() {
			d = driver.Driver
			break
		}
	}
	if d == nil {
		return nil, nil, errors.Errorf("failed getting driver [%s]", configService.VaultPersistenceType())
	}
	logger.Debugf("new fabric vault for channel [%s] with config [%v]", channel, configService)
	persistence, err := db.OpenVersioned(d, db.EscapeForTableName(configService.NetworkName(), channel), db.NewPrefixConfig(configService, configService.VaultPersistencePrefix()))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating vault")
	}

	var txidStore driver.TXIDStore
	txidStore, err = NewTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating txid store")
	}

	txIDStoreCacheSize := configService.VaultTXStoreCacheSize()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed loading txID store cache size from configuration")
	}

	if txIDStoreCacheSize > 0 {
		logger.Debugf("creating txID store second cache with size [%d]", txIDStoreCacheSize)
		c := txidstore.NewCache(txidStore, secondcache.NewTyped[*txidstore.Entry](txIDStoreCacheSize), logger)
		return NewVault(persistence, c, metricsProvider, tracerProvider), c, nil
	} else {
		logger.Debugf("txID store without cache selected")
		c := txidstore.NewNoCache(txidStore)
		return NewVault(persistence, c, metricsProvider, tracerProvider), c, nil
	}
}
