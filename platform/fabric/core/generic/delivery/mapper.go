/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type parallelResponseMapper[O any] struct {
	mapper TxMapper[O]
	cap    int
}

func NewParallelResponseMapper[O any](mapper TxMapper[O], cap int) *parallelResponseMapper[O] {
	return &parallelResponseMapper[O]{
		mapper: mapper,
		cap:    cap,
	}
}

func (m *parallelResponseMapper[O]) Map(response any) ([]O, error) {
	switch r := response.(type) {
	case *pb.DeliverResponse_FilteredBlock:
		eg := errgroup.Group{}
		eg.SetLimit(m.cap)
		results := make([]O, len(r.FilteredBlock.FilteredTransactions))
		for i, tx := range r.FilteredBlock.FilteredTransactions {
			eg.Go(func() error {
				logger.Debugf("transaction [%s] in block [%d]", tx.Txid, r.FilteredBlock.Number)
				event, err := m.mapper.MapFiltered(tx, r.FilteredBlock.Number, driver.TxNum(i))
				if err != nil {
					return err
				}
				results[i] = event
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return nil, err
		}
		return results, nil
	case *pb.DeliverResponse_Block:
		eg := errgroup.Group{}
		eg.SetLimit(m.cap)
		results := make([]O, len(r.Block.Data.Data))
		for i, tx := range r.Block.Data.Data {
			eg.Go(func() error {
				event, err := m.mapper.MapUnfiltered(tx, r.Block.Header.Number, driver.TxNum(i))
				if err != nil {
					return err
				}
				results[i] = event
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return nil, err
		}
		return results, nil
	case *pb.DeliverResponse_Status:
		return nil, errors.Errorf("deliver completed with status (%s)", r.Status)
	default:
		return nil, errors.Errorf("received unexpected response type (%T)", r)
	}
}

type TxMapper[O any] interface {
	MapUnfiltered(tx []byte, blockNum driver.BlockNum, txNum driver.TxNum) (O, error)
	MapFiltered(tx *pb.FilteredTransaction, blockNum driver.BlockNum, txNum driver.TxNum) (O, error)
}

type txMapper struct{}

func (m *txMapper) MapUnfiltered(tx []byte, blockNum driver.BlockNum, txNum driver.TxNum) (TxEvent, error) {
	_, _, chdr, err := fabricutils.UnmarshalTx(tx)
	if err != nil {
		return TxEvent{}, errors.Wrapf(err, "error parsing transaction [%d,%d]", blockNum, txNum)
	}
	event := TxEvent{
		TxID:         chdr.TxId,
		Committed:    true,
		Block:        blockNum,
		IndexInBlock: int(txNum),
		Err:          nil,
	}
	return event, nil
}

func (m *txMapper) MapFiltered(tx *pb.FilteredTransaction, blockNum driver.BlockNum, txNum driver.TxNum) (TxEvent, error) {
	if tx.TxValidationCode == pb.TxValidationCode_VALID {
		logger.Debugf("transaction [%s] in block [%d] is valid", tx.Txid, blockNum)
		return TxEvent{
			TxID:         tx.Txid,
			Committed:    true,
			Block:        blockNum,
			IndexInBlock: int(txNum),
			Err:          nil,
		}, nil
	} else {
		logger.Debugf("transaction [%s] in block [%d] is not valid [%s]", tx.Txid, blockNum, tx.TxValidationCode)
		return TxEvent{
			TxID:         tx.Txid,
			Committed:    false,
			Block:        blockNum,
			IndexInBlock: int(txNum),
			Err:          errors.Errorf("transaction [%s] status is not valid: %s", tx.Txid, tx.TxValidationCode),
		}, nil
	}
}
