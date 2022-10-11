/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/hyperledger-labs/orion-server/pkg/types"
)

type Ledger struct {
	ledger bcdb.Ledger
}

func (l *Ledger) NewBlockHeaderDeliveryService(conf *bcdb.BlockHeaderDeliveryConfig) bcdb.BlockHeaderDelivererService {
	return l.ledger.NewBlockHeaderDeliveryService(conf)
}

func (l *Ledger) GetTransactionReceipt(txId string) (*types.TxReceipt, error) {
	return l.ledger.GetTransactionReceipt(txId)
}
