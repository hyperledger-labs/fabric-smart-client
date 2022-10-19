/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/pkg/errors"
)

type Network interface {
	TransactionManager() driver.TransactionManager
	EnvelopeService() driver.EnvelopeService
}

type Vault struct {
	*vault.Vault
	*vault.SimpleTXIDStore
	network Network
}

func NewVault(sp view.ServiceProvider, config *config.Config, network Network, channel string) (*Vault, error) {
	pType := config.VaultPersistenceType()
	if pType == "file" {
		// for retro compatibility
		pType = "badger"
	}
	persistence, err := db.OpenVersioned(sp, pType, channel, db.NewPrefixConfig(config, config.VaultPersistencePrefix()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault")
	}

	txIDStore, err := vault.NewSimpleTXIDStore(db.Unversioned(persistence))
	if err != nil {
		return nil, err
	}

	return &Vault{
		Vault:           vault.New(persistence, txIDStore),
		SimpleTXIDStore: txIDStore,
		network:         network,
	}, nil
}

func (v *Vault) GetLastTxID() (string, error) {
	return v.SimpleTXIDStore.GetLastTxID()
}

func (v *Vault) NewQueryExecutor() (odriver.QueryExecutor, error) {
	return v.Vault.NewQueryExecutor()
}

func (v *Vault) NewRWSet(txID string) (odriver.RWSet, error) {
	return v.Vault.NewRWSet(txID)
}

func (v *Vault) GetRWSet(id string, results []byte) (odriver.RWSet, error) {
	return v.Vault.GetRWSet(id, results)
}

func (v *Vault) Status(txID string) (odriver.ValidationCode, error) {
	vc, err := v.Vault.Status(txID)
	if err != nil {
		return odriver.Unknown, err
	}
	if vc == odriver.Unknown {
		// give it a second chance
		if v.network.EnvelopeService().Exists(txID) {
			if err := v.extractStoredEnvelopeToVault(txID); err != nil {
				return odriver.Unknown, errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = odriver.Busy
		}
	}
	return vc, nil
}

func (v *Vault) DiscardTx(txID string) error {
	vc, err := v.Vault.Status(txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == odriver.Unknown {
		return nil
	}

	return v.Vault.DiscardTx(txID)
}

func (v *Vault) CommitTX(txID string, block uint64, indexInBloc int) error {
	return v.Vault.CommitTX(txID, block, indexInBloc)
}

func (v *Vault) extractStoredEnvelopeToVault(txID string) error {
	// extract envelope
	envRaw, err := v.network.EnvelopeService().LoadEnvelope(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to load fabric envelope for [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s]", txID)
	env := v.network.TransactionManager().NewEnvelope()
	if err := env.FromBytes(envRaw); err != nil {
		return errors.Wrapf(err, "cannot unmarshal envelope [%s]", txID)
	}
	rws, err := v.GetRWSet(txID, env.Results())
	if err != nil {
		return errors.Wrapf(err, "cannot unmarshal envelope [%s]", txID)
	}
	rws.Done()

	return nil
}
