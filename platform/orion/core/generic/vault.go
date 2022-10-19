/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	odriver "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

type Badger struct {
	Path string
}

type Vault struct {
	*vault.Vault
	*vault.SimpleTXIDStore
	envelopeService driver.EnvelopeService
}

func NewVault(sp view.ServiceProvider, config *config.Config, envelopeService driver.EnvelopeService, channel string) (*Vault, error) {
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
		envelopeService: envelopeService,
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
		if v.envelopeService.Exists(txID) {
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
	envRaw, err := v.envelopeService.LoadEnvelope(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to load fabric envelope for [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s]", txID)
	env := &common.Envelope{}
	err = proto.Unmarshal(envRaw, env)
	if err != nil {
		return errors.Wrapf(err, "failed unmarshalling envelope [%s]", txID)
	}
	logger.Debugf("unpack envelope [%s]", txID)
	upe, err := rwset.UnpackEnvelope("", env)
	if err != nil {
		return errors.Wrapf(err, "failed unpacking envelope [%s]", txID)
	}
	logger.Debugf("retrieve rws [%s]", txID)
	// load into the vault
	rws, err := v.GetRWSet(txID, upe.Results)
	if err != nil {
		return errors.WithMessagef(err, "failed to parse fabric envelope's rws for [%s][%s]", txID, hash.Hashable(envRaw).String())
	}
	rws.Done()

	return nil
}
