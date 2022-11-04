/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/orion-sdk-go/pkg/bcdb"
	"github.com/pkg/errors"
)

type Ledger struct {
	ledger bcdb.Ledger
}

func (l *Ledger) NewBlockHeaderDeliveryService(conf driver.DeliveryServiceConfig) (driver.DeliverStream, error) {
	c, ok := conf.(*bcdb.BlockHeaderDeliveryConfig)
	if !ok {
		return nil, errors.Errorf("expect *bcdb.BlockHeaderDeliveryConfig, got [%T]", conf)
	}

	return l.ledger.NewBlockHeaderDeliveryService(c), nil
}

func (l *Ledger) GetTransactionReceipt(txId string) (driver.Flag, error) {
	r, err := l.ledger.GetTransactionReceipt(txId)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to ger transaction recipet for [%s]", txId)
	}

	return driver.Flag(r.Header.ValidationInfo[r.TxIndex].Flag), nil
}
