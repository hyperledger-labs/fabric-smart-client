/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/fabricx/simple/views/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
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

func (i *CreateView) Call(viewCtx view.Context) (interface{}, error) {
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
	if _, err = viewCtx.RunView(state.NewCollectEndorsementsView(tx, i.params.Approvers...)); err != nil {
		return nil, err
	}

	// create a listener go check when the tx is committed
	network, ch, err := fabric.GetDefaultChannel(viewCtx)
	if err != nil {
		return nil, err
	}

	lm, err := finality.GetListenerManager(viewCtx.Context(), viewCtx, network.Name(), ch.Name())
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	wg.Add(1)

	logger.Infof("Setup finality listener for txID=%v", tx.ID())
	err = lm.AddFinalityListener(tx.ID(), utils.NewFinalityListener(tx.ID(), fdriver.Valid, &wg))
	if err != nil {
		return nil, err
	}

	go func() {
		err := lm.Listen(viewCtx.Context())
		if err != nil && !errors.Is(err, context.Canceled) {
			assert.NoError(err)
		}
	}()

	// now we have a committer listener registered, we send the approved transaction to the orderer
	logger.Infof("Submit tx (txID=%v) to ordering service", tx.ID())
	if _, err = viewCtx.RunView(state.NewOrderingAndFinalityWithTimeoutView(tx, FinalityTimeout)); err != nil {
		return nil, err
	}

	// wait until it is committed
	logger.Infof("Wait for txID=%v to be committed", tx.ID())
	wg.Wait()

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
