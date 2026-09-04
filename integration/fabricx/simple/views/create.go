/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"time"

	views2 "github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

const FinalityTimeout = 30 * time.Second

type CreateParams struct {
	Owner     string
	Value     int
	Namespace string
	//
	Approvers []view.Identity
}

type CreateView struct {
	params CreateParams
}

func (i *CreateView) Call(viewCtx view.Context) (any, error) {
	// this is our state we want to
	obj := &SomeObject{
		Owner: i.params.Owner,
		Value: i.params.Value,
	}

	// create a new transaction
	logger.Infof("Create a new transaction to create a new object with the following parameters: %v", i.params)
	tx, err := state.NewTransaction(viewCtx)
	if err != nil {
		return nil, err
	}

	tx.SetNamespace(i.params.Namespace)

	if err = tx.AddCommand("create"); err != nil {
		return nil, err
	}

	// note that this function produces a new entry in thr write set, generating a key and using obj as value
	if err = tx.AddOutput(obj); err != nil {
		return nil, err
	}

	// send transaction do all approvers
	logger.Infof("Collect endorsements from %v for txID=%v", i.params.Approvers, tx.ID())
	if _, err = viewCtx.RunView(state.NewCollectEndorsementsView(
		tx,
		append(
			// the current node must also endorse because it is generating the RWSet
			[]view.Identity{tx.FabricNetworkService().IdentityProvider().DefaultIdentity()},
			i.params.Approvers...,
		)...,
	)); err != nil {
		return nil, err
	}

	// create a listener go check when the tx is committed
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}

	// Through the committer, not finality.GetListenerManager: only the committer's
	// manager answers for a transaction the committer has already reported on. See
	// platform/fabricx/core/finality.ResolvingListenerManager.
	lm := ch.Committer()

	logger.Infof("Setup finality listener for txID=%v", tx.ID())
	listener := views2.NewFinalityListener(tx.ID())
	if err = lm.AddFinalityListener(tx.ID(), listener); err != nil {
		return nil, err
	}
	defer func() {
		if err := lm.RemoveFinalityListener(tx.ID(), listener); err != nil {
			logger.Warnf("failed to remove finality listener for txID=%v: %v", tx.ID(), err)
		}
	}()

	// now we have a committer listener registered, we send the approved transaction to the orderer
	logger.Infof("Submit tx (txID=%v) to ordering service", tx.ID())
	if _, err = viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)); err != nil {
		return nil, err
	}

	// wait until it is committed
	logger.Infof("Wait for txID=%v to be committed", tx.ID())
	if err := listener.Expect(viewCtx.Context(), fdriver.Valid, FinalityTimeout); err != nil {
		return nil, err
	}

	// exercise the ledger service
	lp, err := ledger.GetLedgerProvider(viewCtx)
	if err != nil {
		return nil, err
	}
	l, err := lp.NewLedger(network.Name(), ch.Name())
	if err != nil {
		return nil, err
	}

	info, err := l.GetLedgerInfo()
	if err != nil {
		return nil, err
	}
	logger.Infof("Ledger info: height=%v", info.Height)

	// The block index lags finality -- see ledger.GetTransactionByID.
	pt, err := utils.NewTypedRetryRunner[fdriver.ProcessedTransaction](30, 500*time.Millisecond, false).
		Run(func() (fdriver.ProcessedTransaction, error) { return l.GetTransactionByID(tx.ID()) })
	if err != nil {
		return nil, err
	}
	logger.Infof("Transaction found: txID=%v, valid=%v, code=%v", pt.TxID(), pt.IsValid(), pt.ValidationCode())
	if !pt.IsValid() {
		return nil, errors.Errorf("tx [%s] is invalid", pt.TxID())
	}

	blockNum, err := l.GetBlockNumberByTxID(tx.ID())
	if err != nil {
		return nil, err
	}
	logger.Infof("Transaction committed in block %v", blockNum)

	return nil, nil
}

type CreateViewFactory struct{}

func (*CreateViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateView{}
	if err := json.Unmarshal(in, &f.params); err != nil {
		return nil, err
	}

	return f, nil
}
