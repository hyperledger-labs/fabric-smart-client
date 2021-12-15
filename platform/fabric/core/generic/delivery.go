/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/protoutil"
)

func (c *channel) Scan(ctx context.Context, txID string, callback driver.DeliveryCallback) error {
	vault := &fakeVault{txID: txID}
	deliveryService, err := delivery2.New(
		ctx,
		c.name,
		c.sp,
		c.network,
		func(block *common.Block) (bool, error) {
			for _, tx := range block.Data.Data {
				chHdr, err := protoutil.UnmarshalChannelHeader(tx)
				if err != nil {
					return false, err
				}

				if common.HeaderType(chHdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
					continue
				}

				ptx, err := newProcessedTransactionFromEnvelopeRaw(tx)
				if err != nil {
					return false, err
				}

				stop, err := callback(ptx)
				if err != nil {
					// if an error occurred, stop processing
					return false, err
				}
				if stop {
					return true, nil
				}
				vault.txID = chHdr.TxId
				logger.Debugf("commit transaction [%s] in block [%d]", chHdr.TxId, block.Header.Number)
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
