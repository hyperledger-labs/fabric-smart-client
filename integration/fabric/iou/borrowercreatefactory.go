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

type CreateIOUInitiatorViewFactory struct{}

func (c *CreateIOUInitiatorViewFactory) NewView(in []byte) (view.View, error) {
	f := &CreateIOUInitiatorView{}
	err := json.Unmarshal(in, &f.Create)
	assert.NoError(err)
	return f, nil
}
