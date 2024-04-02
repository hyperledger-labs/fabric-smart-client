/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
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

	StatusReporters []driver.StatusReporter
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

func (v *Vault) NewQueryExecutor() (driver.QueryExecutor, error) {
	return v.Vault.NewQueryExecutor()
}

func (v *Vault) NewRWSet(txID string) (driver.RWSet, error) {
	return v.Vault.NewRWSet(txID)
}

func (v *Vault) GetRWSet(id string, results []byte) (driver.RWSet, error) {
	return v.Vault.GetRWSet(id, results)
}

func (v *Vault) Status(txID string) (driver.ValidationCode, string, error) {
	vc, message, err := v.Vault.Status(txID)
	if err != nil {
		return driver.Unknown, "", err
	}
	if vc == driver.Unknown {
		// give it a second chance
		if v.network.EnvelopeService().Exists(txID) {
			if err := v.extractStoredEnvelopeToVault(txID); err != nil {
				return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = driver.Busy
		} else {
			// check status reporter, if any
			for _, reporter := range v.StatusReporters {
				externalStatus, externalMessage, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					vc = externalStatus
					message = externalMessage
				}
			}
		}
	}
	return vc, message, nil
}

func (v *Vault) DiscardTx(txID string, message string) error {
	vc, _, err := v.Vault.Status(txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == driver.Unknown {
		// check status reporter, if any
		found := false
		for _, reporter := range v.StatusReporters {
			externalStatus, _, _, err := reporter.Status(txID)
			if err == nil && externalStatus != driver.Unknown {
				found = true
				break
			}
		}
		if !found {
			logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
			return nil
		}
	}

	return v.Vault.DiscardTx(txID, message)
}

func (v *Vault) CommitTX(txID string, block uint64, indexInBloc int) error {
	return v.Vault.CommitTX(txID, block, indexInBloc)
}

func (v *Vault) AddStatusReporter(sr driver.StatusReporter) error {
	v.StatusReporters = append(v.StatusReporters, sr)
	return nil
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
