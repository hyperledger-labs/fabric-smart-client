/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mock

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// InitiatorViewFactory is the factory of Initiator views
type InitiatorViewFactory struct{}

// NewView returns a new instance of the Initiator view
func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	f := &Initiator{Params: &Params{}}
	err := json.Unmarshal(in, f.Params)
	assert.NoError(err, "failed unmarshalling input")
	return f, nil
}
