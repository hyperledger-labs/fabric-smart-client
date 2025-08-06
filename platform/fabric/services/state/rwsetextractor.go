/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type Network interface {
	Channel(name string) (*fabric.Channel, error)
}

type RWSetProcessor struct {
	network Network
}

func NewRWSetProcessor(network Network) *RWSetProcessor {
	return &RWSetProcessor{network: network}
}

func (r *RWSetProcessor) Process(req fabric.Request, tx fabric.ProcessTransaction, rws *fabric.RWSet, ns string) error {
	txID := tx.ID()

	ch, err := r.network.Channel(tx.Channel())
	if err != nil {
		return errors.Wrapf(err, "failed getting channel [%s]", tx.Channel())
	}

	if !ch.MetadataService().Exists(context.Background(), txID) {
		logger.Debugf("transaction [%s] is not known to this node, no need to extract state information", txID)
		return nil
	}

	logger.Debugf("transaction [%s] is known, extract state information", txID)

	logger.Debugf("transaction [%s], parsing writes [%d]", txID, rws.NumWrites(ns))
	for i := 0; i < rws.NumWrites(ns); i++ {
		key, _, err := rws.GetWriteAt(ns, i)
		if err != nil {
			return err
		}
		logger.Debugf("Parsing write key [%s]", key)

		meta, err := rws.GetStateMetadata(ns, key, driver.FromIntermediate)
		if err != nil {
			return errors.Wrap(err, "filed getting metadata")
		}
		if len(meta) == 0 {
			meta = map[string][]byte{}
		}

		// extrate state info from metadata service
		transientMap, err := ch.MetadataService().LoadTransient(context.Background(), txID)
		if err != nil {
			return err
		}
		k, err := fieldMappingKey(ns, key)
		if err != nil {
			panic("filed creating mapping key")
		}
		if len(transientMap[k]) != 0 {
			meta[k] = transientMap[k]
		} else {
			continue
		}

		logger.Debugf("Set metadata for key [%s][%v]", key, string(meta[k]))
		err = rws.SetStateMetadata(ns, key, meta)
		if err != nil {
			return err
		}
	}
	logger.Debugf("transaction [%s], parsing writes [%d] done", txID, rws.NumWrites(ns))

	return nil
}
