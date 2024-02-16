/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

type RWSetLoader struct {
	Network            string
	Channel            string
	EnvelopeService    driver.EnvelopeService
	TransactionService driver.EndorserTransactionService
	TransactionManager driver.TransactionManager

	Vault *vault.Vault
}

func NewRWSetLoader(network string, channel string, envelopeService driver.EnvelopeService, transactionService driver.EndorserTransactionService, transactionManager driver.TransactionManager, vault *vault.Vault) driver.RWSetLoader {
	return &RWSetLoader{Network: network, Channel: channel, EnvelopeService: envelopeService, TransactionService: transactionService, TransactionManager: transactionManager, Vault: vault}
}

func (c *RWSetLoader) GetRWSetFromEvn(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	rawEnv, err := c.EnvelopeService.LoadEnvelope(txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load envelope [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s,%s]", c.Channel, txID)
	env := &common.Envelope{}
	err = proto.Unmarshal(rawEnv, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unmarshalling envelope [%s]", txID)
	}
	logger.Debugf("unpack envelope [%s,%s]", c.Channel, txID)
	upe, err := rwset.UnpackEnvelope(c.Network, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unpacking envelope [%s]", txID)
	}
	logger.Debugf("retrieve rws [%s,%s]", c.Channel, txID)

	rws, err := c.Vault.GetRWSet(txID, upe.Results)
	if err != nil {
		return nil, nil, err
	}

	return rws, upe, nil
}

func (c *RWSetLoader) GetRWSetFromETx(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	raw, err := c.TransactionService.LoadTransaction(txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load etx [%s]", txID)
	}
	tx, err := c.TransactionManager.NewTransactionFromBytes(c.Channel, raw)
	if err != nil {
		return nil, nil, err
	}
	rws, err := tx.GetRWSet()
	if err != nil {
		return nil, nil, err
	}
	return rws, tx, nil
}
