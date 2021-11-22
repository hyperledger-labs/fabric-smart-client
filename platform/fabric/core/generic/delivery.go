/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"strings"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"

	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func (c *channel) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	deliveryService, err := delivery2.New(
		ctx,
		c.name,
		c.sp,
		c.network,
		func(filteredBlock *peer.FilteredBlock) (bool, error) {
			ledger, err := c.network.Ledger(c.name)
			if err != nil {
				logger.Panicf("cannot get ledger [%s]", err)
			}
			var block driver.Block
			block, err = ledger.GetBlockByNumber(filteredBlock.Number)
			if err != nil {
				if !strings.Contains(err.Error(), "grpc: trying to send message larger than max") {
					logger.Debugf("cannot get filteredBlock [%s]", err)
					return true, err
				}

				// The block is too big, download each transaction as needed
				logger.Warnf("block [%d] too big to be downloaded, it contains [%d] txs",
					filteredBlock.Number,
					len(filteredBlock.FilteredTransactions))
				block = nil
			}

			filteredTransactions := filteredBlock.FilteredTransactions
			for i, tx := range filteredTransactions {
				logger.Debugf("commit transaction [%s] in filteredBlock [%d]", tx.Txid, filteredBlock.Number)

				switch tx.Type {
				case common.HeaderType_CONFIG:
					// do nothing, for now
				case common.HeaderType_ENDORSER_TRANSACTION:
					tx := filteredTransactions[i]
					switch tx.TxValidationCode {
					case peer.TxValidationCode_VALID:
						ptx, err := block.ProcessedTransaction(i)
						if err != nil {
							logger.Panicf("cannot get processed transaction [%s]", err)
						}
						stop, err := callback(ptx)
						if err != nil {
							// if an error occurred, stop processing
							return false, err
						}
						if stop {
							return true, nil
						}
						vault.txID = tx.Txid
					}
				}
			}
			return false, nil
		},
		vault,
		waitForEventTimeout,
	)
	if err != nil {
		return err
	}

	return deliveryService.Run()
}

type fakeVault struct {
	txID string
}

func (f *fakeVault) GetLastTxID() (string, error) {
	return f.txID, nil
}
