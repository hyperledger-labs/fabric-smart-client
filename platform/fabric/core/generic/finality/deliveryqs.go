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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/events"
)

type DeliveryScanQueryByID[T events.EventInfo] struct {
	Delivery *fabric.Delivery
	Mapper   events.EventInfoMapper[T]
}

func (q *DeliveryScanQueryByID[T]) QueryByID(ctx context.Context, lastBlock driver.BlockNum, evicted map[driver.TxID][]events.ListenerEntry[T]) (<-chan []T, error) {
	txIDs := collections.Keys(evicted)
	results := collections.NewSet(txIDs...)
	ch := make(chan []T, len(txIDs))
	go q.queryByID(ctx, results, ch, lastBlock)
	return ch, nil
}

func (q *DeliveryScanQueryByID[T]) queryByID(ctx context.Context, results collections.Set[string], ch chan []T, lastBlock uint64) {
	defer close(ch)

	startingBlock := MaxUint64(1, lastBlock-10)
	err := q.Delivery.ScanFromBlock(ctx, startingBlock, func(tx *fabric.ProcessedTransaction) (bool, error) {
		if !results.Contains(tx.TxID()) {
			return false, nil
		}

		logger.Debugf("Received result for tx [%s, %v, %d]...", tx.TxID(), tx.ValidationCode(), len(tx.Results()))
		infos, err := q.Mapper.MapProcessedTx(tx)
		if err != nil {
			logger.Errorf("failed mapping tx [%s]: %v", tx.TxID(), err)
			return true, err
		}
		ch <- infos
		results.Remove(tx.TxID())

		return results.Length() == 0, nil
	})
	if err != nil {
		logger.Errorf("failed scanning: %v", err)
	}
}

func MaxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
