/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
)

type TXIDStore interface {
	fdriver.TXIDStore
	Get(txid string) (fdriver.ValidationCode, error)
	Set(txid string, code fdriver.ValidationCode) error
}

func NewVault(sp view2.ServiceProvider, config *config.Config, channel string, cacheSize int) (*vault.Vault, TXIDStore, error) {
	pType := config.VaultPersistenceType()
	if pType == "file" {
		// for retro compatibility
		pType = "badger"
	}
	persistence, err := db.OpenVersioned(sp, pType, channel, db.NewPrefixConfig(config, config.VaultPersistencePrefix()))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating vault")
	}

	var txidStore TXIDStore
	txidStore, err = txidstore.NewTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating txid store")
	}

	if cacheSize > 0 {
		logger.Debugf("creating txID store second cache with size [%d]", cacheSize)
		txidStore = txidstore.NewCache(txidStore, secondcache.New(cacheSize))
	}

	return vault.New(persistence, txidStore), txidStore, nil
}
