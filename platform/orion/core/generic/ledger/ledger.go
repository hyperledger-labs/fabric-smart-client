/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ledger

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/pkg/errors"
)

type DeliverStream interface {
	Receive() interface{}
	Stop()
	Error() error
}

type Ledger struct {
	ledger bcdb.Ledger
}

func NewLedger(ledger bcdb.Ledger) *Ledger {
	return &Ledger{ledger: ledger}
}

func (l *Ledger) NewBlockHeaderDeliveryService(conf *bcdb.BlockHeaderDeliveryConfig) (DeliverStream, error) {
	return l.ledger.NewBlockHeaderDeliveryService(conf), nil
}

func (l *Ledger) GetTransactionReceipt(txId string) (driver.Flag, error) {
	r, err := l.ledger.GetTransactionReceipt(txId)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get transaction recipet for [%s]", txId)
	}

	return driver.Flag(r.Header.ValidationInfo[r.TxIndex].Flag), nil
}
