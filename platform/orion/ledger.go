/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/pkg/errors"
)

type Flag = driver.Flag

const (
	VALID = driver.VALID
	INVALID_MVCC_CONFLICT_WITHIN_BLOCK
	INVALID_MVCC_CONFLICT_WITH_COMMITTED_STATE
	INVALID_DATABASE_DOES_NOT_EXIST
	INVALID_NO_PERMISSION
	INVALID_INCORRECT_ENTRIES
	INVALID_UNAUTHORISED
	INVALID_MISSING_SIGNATURE
)

// ProcessedTransaction models a transaction that has been processed by Fabric
type ProcessedTransaction struct {
	txID string
	vc   Flag
}

// TxID returns the transaction's id
func (pt *ProcessedTransaction) TxID() string {
	return pt.txID
}

// ValidationCode returns the transaction's validation code
func (pt *ProcessedTransaction) ValidationCode() Flag {
	return pt.vc
}

type Ledger struct {
	l driver.Ledger
}

func (l *Ledger) GetTransactionByID(txID string) (*ProcessedTransaction, error) {
	flag, err := l.l.GetTransactionReceipt(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get transaction receipt for [%s]", txID)
	}
	return &ProcessedTransaction{
		txID: txID,
		vc:   flag,
	}, nil
}
