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

type QueryViewFactory struct{}

func (c *QueryViewFactory) NewView(in []byte) (view.View, error) {
	f := &QueryView{}
	err := json.Unmarshal(in, &f.Query)
	assert.NoError(err)
	return f, nil
}
