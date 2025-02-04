/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type DeliveryScanQueryByID[T TxInfo] struct {
	delivery *fabric.Delivery
	mapper   TxInfoMapper[T]
}

func (q *DeliveryScanQueryByID[T]) QueryByID(txIDs ...driver.TxID) (<-chan []T, error) {
	evicted := collections.NewSet(txIDs...)
	ch := make(chan []T, len(txIDs))

	err := q.delivery.Scan(context.TODO(), "", func(tx *fabric.ProcessedTransaction) (bool, error) {
		if !evicted.Contains(tx.TxID()) {
			return false, nil
		}

		logger.Debugf("Received result for tx [%s, %v, %d]...", tx.TxID(), tx.ValidationCode(), len(tx.Results()))
		infos, err := q.mapper.MapProcessedTx(tx)
		if err != nil {
			logger.Errorf("failed mapping tx [%s]: %v", tx.TxID(), err)
			return true, err
		}
		ch <- infos
		evicted.Remove(tx.TxID())

		return evicted.Length() == 0, nil
	})
	if err != nil {
		logger.Errorf("Failed scanning: %v", err)
		return nil, err
	}

	return ch, nil
}
