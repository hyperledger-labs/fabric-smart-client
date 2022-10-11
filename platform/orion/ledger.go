/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/pkg/errors"
)

type Flag int32

const (
	Flag_VALID                                      Flag = 0
	Flag_INVALID_MVCC_CONFLICT_WITHIN_BLOCK         Flag = 1
	Flag_INVALID_MVCC_CONFLICT_WITH_COMMITTED_STATE Flag = 2
	Flag_INVALID_DATABASE_DOES_NOT_EXIST            Flag = 3
	Flag_INVALID_NO_PERMISSION                      Flag = 4
	Flag_INVALID_INCORRECT_ENTRIES                  Flag = 5
	Flag_INVALID_UNAUTHORISED                       Flag = 6
	Flag_INVALID_MISSING_SIGNATURE                  Flag = 7
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
func (pt *ProcessedTransaction) ValidationCode() string {
	return pt.txID
}

type Ledger struct {
	l driver.Ledger
}

func (l *Ledger) GetTransactionByID(txID string) (*ProcessedTransaction, error) {
	r, err := l.l.GetTransactionReceipt(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get transaction receipt for [%s]", txID)
	}
	return &ProcessedTransaction{
		txID: txID,
		vc:   Flag(r.Header.ValidationInfo[r.TxIndex].Flag),
	}, nil
}
