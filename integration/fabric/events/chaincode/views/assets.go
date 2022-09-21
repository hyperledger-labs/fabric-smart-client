/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type CreateAsset struct {
	Asset Asset
}

type CreateAssetView struct {
	*CreateAsset
}

var logger = flogging.MustGetLogger("assets")

func (c *CreateAssetView) Call(context view.Context) (interface{}, error) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Register for events
	callBack := func(event *committer.ChaincodeEvent) (bool, error) {
		logger.Debugf("Event Received in callback ", event)
		// TODO: check the event is as expected
		wg.Done()
		return true, nil
	}
	_, err := context.RunView(chaincode.NewListenToEventsView("asset_transfer_events", callBack))
	assert.NoError(err, "failed to listen to events")

	// Invoke the chaincode
	_, err = context.RunView(
		chaincode.NewInvokeView(
			"asset_transfer_events",
			"CreateAsset",
			c.Asset.ID,
			c.Asset.Color,
			c.Asset.Size,
			c.Asset.Owner,
			c.Asset.AppraisedValue,
		),
	)
	assert.NoError(err, "failed creating asset")

	// wait for the event to arriver
	wg.Wait()

	return nil, nil
}

type CreateAssetViewFactory struct{}

func (c *CreateAssetViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateAssetView{CreateAsset: &CreateAsset{}}
	err := json.Unmarshal(in, f.CreateAsset)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
