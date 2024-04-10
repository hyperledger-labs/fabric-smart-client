/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/pkg/errors"
)

type TXIDStore interface {
	fdriver.TXIDStore
	Get(txid string) (fdriver.ValidationCode, string, error)
	Set(txID string, code fdriver.ValidationCode, message string) error
}

func NewVault(sp view2.ServiceProvider, configService fdriver.ConfigService, channel string) (*vault.Vault, TXIDStore, error) {
	logger.Debugf("new fabric vault for channel [%s] with config [%v]", channel, configService)
	pType := configService.VaultPersistenceType()
	if pType == "file" {
		// for retro compatibility
		pType = "badger"
	}
	persistence, err := db.OpenVersioned(sp, pType, channel, db.NewPrefixConfig(configService, configService.VaultPersistencePrefix()))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating vault")
	}

	var txidStore TXIDStore
	txidStore, err = txidstore.NewTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating txid store")
	}

	txIDStoreCacheSize := configService.VaultTXStoreCacheSize()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed loading txID store cache size from configuration")
	}

	if txIDStoreCacheSize > 0 {
		logger.Debugf("creating txID store second cache with size [%d]", txIDStoreCacheSize)
		txidStore = txidstore.NewCache(txidStore, secondcache.NewTyped[*txidstore.Entry](txIDStoreCacheSize))
	}

	return vault.New(persistence, txidStore), txidStore, nil
}
