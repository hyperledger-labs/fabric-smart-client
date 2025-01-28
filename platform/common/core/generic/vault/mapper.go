/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/pkg/errors"
)

type VersionBuilder interface {
	VersionedValues(rws *ReadWriteSet, ns driver.Namespace, writes NamespaceWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]VersionedValue, error)
	VersionedMetaValues(rws *ReadWriteSet, ns driver.Namespace, writes KeyedMetaWrites, block driver.BlockNum, indexInBloc driver.TxNum) (map[driver.PKey]driver.VersionedMetadataValue, error)
}

type rwSetMapper struct {
	logger Logger
	vb     VersionBuilder
}

func (m *rwSetMapper) mapTxIDs(inputs []commitInput) []driver.TxID {
	txIDs := make([]driver.TxID, len(inputs))
	for i, input := range inputs {
		txIDs[i] = input.txID
	}
	return txIDs
}

func (m *rwSetMapper) mapWrites(inputs []commitInput) (driver.Writes, error) {
	writes := make(map[driver.Namespace]map[driver.PKey]VersionedValue)
	for i, input := range inputs {
		m.logger.Debugf("input [%d] has [%d] writes", i, len(input.rws.Writes))
		for ns, ws := range input.rws.Writes {
			vals, err := m.vb.VersionedValues(input.rws, ns, ws, input.block, input.indexInBloc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse writes for txid %s", input.txID)
			}
			if nsWrites, ok := writes[ns]; !ok {
				writes[ns] = vals
			} else {
				collections.CopyMap(nsWrites, vals)
			}
		}
	}
	return writes, nil
}

func (m *rwSetMapper) mapMetaWrites(inputs []commitInput) (driver.MetaWrites, error) {
	metaWrites := make(map[driver.Namespace]map[driver.PKey]driver.VersionedMetadataValue)
	for _, input := range inputs {
		for ns, ws := range input.rws.MetaWrites {
			vals, err := m.vb.VersionedMetaValues(input.rws, ns, ws, input.block, input.indexInBloc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse metadata writes for txid %s", input.txID)
			}
			if nsWrites, ok := metaWrites[ns]; !ok {
				metaWrites[ns] = vals
			} else {
				collections.CopyMap(nsWrites, vals)
			}
		}
	}
	return metaWrites, nil
}
