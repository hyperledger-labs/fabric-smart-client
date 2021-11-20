/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type Badger struct {
	Path string
}

type Vault struct {
	*vault.Vault
	*vault.SimpleTXIDStore
}

func (v *Vault) GetLastTxID() (string, error) {
	return v.SimpleTXIDStore.GetLastTxID()
}

func (v *Vault) NewQueryExecutor() (odriver.QueryExecutor, error) {
	return v.Vault.NewQueryExecutor()
}

func (v *Vault) NewRWSet(txid string) (odriver.RWSet, error) {
	return v.Vault.NewRWSet(txid)
}

func (v *Vault) GetRWSet(id string, results []byte) (odriver.RWSet, error) {
	return v.Vault.GetRWSet(id, results)
}

func NewVault(config *config.Config, channel string, sp view.ServiceProvider) (*Vault, error) {
	var persistence driver.VersionedPersistence
	pType := config.VaultPersistenceType()
	switch pType {
	case "file":
		opts := &Badger{}
		err := config.VaultPersistenceOpts(opts)
		if err != nil {
			return nil, errors.Wrapf(err, "failed getting opts for vault")
		}
		opts.Path = filepath.Join(opts.Path, channel)
		err = os.MkdirAll(opts.Path, 0755)
		if err != nil {
			return nil, errors.Wrapf(err, "failed creating folders for vault [%s]", opts.Path)
		}
		persistence, err = db.OpenVersioned("badger", opts.Path)
		if err != nil {
			return nil, err
		}
	case "memory":
		var err error
		persistence, err = db.OpenVersioned("memory", "")
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("invalid persistence type, expected one of [file,memory], got [%s]", pType)
	}

	txidstore, err := vault.NewSimpleTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, err
	}

	return &Vault{vault.New(persistence, txidstore), txidstore}, nil
}
