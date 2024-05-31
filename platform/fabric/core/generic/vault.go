/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/pkg/errors"
)

type VaultService struct {
	*vault.Vault
}

func NewVaultService(vault *vault.Vault) *VaultService {
	return &VaultService{Vault: vault}
}

// NewRWSet returns a RWSet for this ledger.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *VaultService) NewRWSet(txid string) (driver.RWSet, error) {
	return c.Vault.NewRWSet(txid)
}

// GetRWSet returns a RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// A client may obtain more than one such simulator; they are made unique
// by way of the supplied txid
func (c *VaultService) GetRWSet(txid string, rwset []byte) (driver.RWSet, error) {
	return c.Vault.GetRWSet(txid, rwset)
}

// GetEphemeralRWSet returns an ephemeral RWSet for this ledger whose content is unmarshalled
// from the passed bytes.
// If namespaces is not empty, the returned RWSet will be filtered by the passed namespaces
func (c *VaultService) GetEphemeralRWSet(rwset []byte, namespaces ...string) (driver.RWSet, error) {
	return c.Vault.InspectRWSet(rwset, namespaces...)
}

type TXIDStore interface {
	driver.TXIDStore
	Get(txid string) (driver.ValidationCode, string, error)
	Set(txID string, code driver.ValidationCode, message string) error
}

func NewVault(configService driver.ConfigService, channel string) (*vault.Vault, TXIDStore, error) {
	logger.Debugf("new fabric vault for channel [%s] with config [%v]", channel, configService)
	pType := configService.VaultPersistenceType()
	if pType == "file" {
		// for retro compatibility
		pType = "badger"
	}
	persistence, err := db.OpenVersioned(pType, db.EscapeForTableName(configService.NetworkName(), channel), db.NewPrefixConfig(configService, configService.VaultPersistencePrefix()))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed creating vault")
	}

	var txidStore TXIDStore
	txidStore, err = vault.NewTXIDStore(db.Unversioned(persistence))
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
		return vault.New(persistence, c), c, nil
	} else {
		logger.Debugf("txID store without cache selected")
		c := txidstore.NewNoCache(txidStore)
		return vault.New(persistence, c), c, nil
	}
}
