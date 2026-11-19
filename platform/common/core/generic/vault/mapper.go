/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
)

type VersionBuilder interface {
	VersionedValues(rws *ReadWriteSet, ns driver.Namespace, writes NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultValue, error)
	VersionedMetaValues(rws *ReadWriteSet, ns driver.Namespace, writes KeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VaultMetadataValue, error)
}

type rwSetMapper struct {
	logger Logger
	vb     VersionBuilder
}

// mapTxWrites returns the versioned writes and metadata writes of each input,
// one entry per transaction, in the order of inputs.
func (m *rwSetMapper) mapTxWrites(inputs []commitInput) ([]driver.TxWrites, error) {
	txs := make([]driver.TxWrites, len(inputs))
	for i, input := range inputs {
		m.logger.Debugf("input [%d] has [%d] writes", i, len(input.rws.Writes))
		txs[i] = driver.TxWrites{
			TxID:       input.txID,
			Writes:     make(driver.Writes, len(input.rws.Writes)),
			MetaWrites: make(driver.MetaWrites, len(input.rws.MetaWrites)),
		}
		for ns, ws := range input.rws.Writes {
			vals, err := m.vb.VersionedValues(input.rws, ns, ws, input.block, input.indexInBloc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse writes for txid %s", input.txID)
			}
			txs[i].Writes[ns] = vals
		}
		for ns, ws := range input.rws.MetaWrites {
			vals, err := m.vb.VersionedMetaValues(input.rws, ns, ws, input.block, input.indexInBloc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse metadata writes for txid %s", input.txID)
			}
			txs[i].MetaWrites[ns] = vals
		}
	}
	return txs, nil
}
