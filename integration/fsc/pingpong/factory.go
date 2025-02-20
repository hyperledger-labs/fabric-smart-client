/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// InitiatorViewFactory is the factory of Initiator views
type InitiatorViewFactory struct{}

// NewView returns a new instance of the Initiator view
func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	m := Message{}
	if len(in) != 0 {
		if err := json.Unmarshal(in, &m); err != nil {
			return nil, err
		}
	}
	return &Initiator{Message: m}, nil
}
