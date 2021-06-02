/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package iou

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type UpdateIOUInitiatorViewFactory struct{}

func (c *UpdateIOUInitiatorViewFactory) NewView(in []byte) (view.View, error) {
	f := &UpdateIOUInitiatorView{}
	err := json.Unmarshal(in, &f.Update)
	assert.NoError(err)
	return f, nil
}
