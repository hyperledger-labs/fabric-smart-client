/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/pkg/errors"
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

func (v *Vault) Status(txID string) (odriver.ValidationCode, error) {
	return v.Vault.Status(txID)
}

func (v *Vault) DiscardTx(txid string) error {
	vc, err := v.Vault.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status in state db [%s]", txid)
	}
	if vc == odriver.Unknown {
		return nil
	}

	return v.Vault.DiscardTx(txid)
}

func (v *Vault) CommitTX(txid string, block uint64, indexInBloc int) error {
	return v.Vault.CommitTX(txid, block, indexInBloc)
}

func NewVault(sp view.ServiceProvider, config *config.Config, channel string) (*Vault, error) {
	pType := config.VaultPersistenceType()
	if pType == "file" {
		// for retro compatibility
		pType = "badger"
	}
	persistence, err := db.OpenVersioned(sp, pType, channel, db.NewPrefixConfig(config, config.VaultPersistencePrefix()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault")
	}

	txidstore, err := vault.NewSimpleTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, err
	}

	return &Vault{vault.New(persistence, txidstore), txidstore}, nil
}
