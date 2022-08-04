/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	defaultCacheSize = 100
)

type TXIDStore interface {
	fdriver.TXIDStore
	Get(txid string) (fdriver.ValidationCode, error)
	Set(txid string, code fdriver.ValidationCode) error
}

type Badger struct {
	Path string
}

func NewVault(config *config.Config, channel string) (*vault.Vault, TXIDStore, error) {
	var persistence driver.VersionedPersistence
	pType := config.VaultPersistenceType()
	switch pType {
	case "file":
		opts := &Badger{}
		err := config.VaultPersistenceOpts(opts)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed getting opts for vault")
		}
		opts.Path = filepath.Join(opts.Path, channel)
		err = os.MkdirAll(opts.Path, 0755)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed creating folders for vault [%s]", opts.Path)
		}
		persistence, err = db.OpenVersioned("badger", opts.Path)
		if err != nil {
			return nil, nil, err
		}
	case "memory":
		var err error
		persistence, err = db.OpenVersioned("memory", "")
		if err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, errors.Errorf("invalid persistence type, expected one of [file,memory], got [%s]", pType)
	}

	var txidStore TXIDStore
	txidStore, err := txidstore.NewTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating txid store")
	}

	txIDStoreCacheSize := config.VaultTXStoreCacheSize(defaultCacheSize)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed loading txID store cache size from configuration")
	}

	if txIDStoreCacheSize > 0 {
		logger.Debugf("creating txID store second cache with size [%d]", txIDStoreCacheSize)
		txidStore = txidstore.NewCache(txidStore, secondcache.New(txIDStoreCacheSize))
	}

	return vault.New(persistence, txidStore), txidStore, nil
}
