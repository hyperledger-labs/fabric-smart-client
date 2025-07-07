/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Finality struct {
	TxID    string
	Network string
	Channel string
}

type FinalityView struct {
	*Finality
}

func (a *FinalityView) Call(context view.Context) (interface{}, error) {
	_, ch, err := fabric.GetChannel(context, a.Network, a.Channel)
	assert.NoError(err, "failed getting channel [%s:%s]", a.Network, a.Channel)
	err = ch.Finality().IsFinal(context.Context(), a.TxID)
	return nil, err
}

type FinalityViewFactory struct{}

func (c *FinalityViewFactory) NewView(in []byte) (view.View, error) {
	f := &FinalityView{Finality: &Finality{}}
	err := json.Unmarshal(in, f.Finality)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
