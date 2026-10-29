/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Ledger is the input of LedgerView.
type Ledger struct {
	// TxID is a committed transaction.
	TxID string
	// Network and Channel select the channel; empty selects the default.
	Network string
	Channel string
	// UnknownTxID is a transaction ID the ledger does not hold.
	UnknownTxID string
}

// LedgerResult is what LedgerView returns.
type LedgerResult struct {
	// BlockNumber is the block GetBlockNumberByTxID reports for TxID.
	BlockNumber uint64
	// ValidationCode is the code GetTransactionByID reports for TxID, a peer.TxValidationCode.
	ValidationCode int32
	// LedgerHeight is the height GetLedgerInfo reports.
	LedgerHeight uint64
	// BlockHasTx is true if the block GetBlockByNumber returns for BlockNumber contains TxID.
	BlockHasTx bool
	// UnknownTxErr is the error GetTransactionByID returns for UnknownTxID.
	UnknownTxErr string
}

// LedgerView reads a committed transaction back through every method of fabric.Ledger,
// which queries a peer of the node's organization.
type LedgerView struct {
	*Ledger
}

func (v *LedgerView) Call(viewCtx view.Context) (any, error) {
	fns, ch, err := fabric.GetChannel(viewCtx, v.Network, v.Channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting channel [%s:%s]", v.Network, v.Channel)
	}
	l := ch.Ledger()
	res := &LedgerResult{}

	if res.BlockNumber, err = l.GetBlockNumberByTxID(v.TxID); err != nil {
		return nil, errors.WithMessagef(err, "failed getting the block number of [%s]", v.TxID)
	}
	pt, err := l.GetTransactionByID(v.TxID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting transaction [%s]", v.TxID)
	}
	if pt.TxID() != v.TxID {
		return nil, errors.Errorf("asked for transaction [%s], got [%s]", v.TxID, pt.TxID())
	}
	res.ValidationCode = pt.ValidationCode()
	info, err := l.GetLedgerInfo()
	if err != nil {
		return nil, errors.WithMessage(err, "failed getting ledger info")
	}
	res.LedgerHeight = info.Height

	block, err := l.GetBlockByNumber(res.BlockNumber)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting block [%d]", res.BlockNumber)
	}
	for i := 0; block.DataAt(i) != nil; i++ {
		env := fns.TransactionManager().NewEnvelope()
		if err := env.FromBytes(block.DataAt(i)); err != nil {
			return nil, errors.WithMessagef(err, "failed unpacking envelope [%d] of block [%d]", i, res.BlockNumber)
		}
		if env.TxID() == v.TxID {
			res.BlockHasTx = true
			break
		}
	}

	if _, err := l.GetTransactionByID(v.UnknownTxID); err != nil {
		res.UnknownTxErr = err.Error()
	}
	return res, nil
}

type LedgerViewFactory struct{}

func (*LedgerViewFactory) NewView(in []byte) (view.View, error) {
	f := &LedgerView{Ledger: &Ledger{}}
	if err := json.Unmarshal(in, f.Ledger); err != nil {
		return nil, err
	}
	return f, nil
}
