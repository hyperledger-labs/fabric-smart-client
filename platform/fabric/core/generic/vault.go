/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault/txidstore"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type Badger struct {
	Path string
}

func NewVault(config *Config, channel string, sp view.ServiceProvider) (*vault.Vault, *txidstore.TXIDStore, error) {
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

	txidstore, err := txidstore.NewTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, nil, err
	}

	return vault.New(persistence, txidstore), txidstore, nil
}
