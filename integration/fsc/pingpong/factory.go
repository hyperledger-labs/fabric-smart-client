/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// InitiatorViewFactory is the factory of Initiator views
type InitiatorViewFactory struct{}

// NewView returns a new instance of the Initiator view
func (i *InitiatorViewFactory) NewView(in []byte) (view.View, error) {
	initiator := &Initiator{}
	if len(in) > 0 {
		if err := json.Unmarshal(in, &initiator.Params); err != nil {
			return nil, errors.Wrapf(err, "failed unmarshalling input [%s]", string(in))
		}
	}
	return initiator, nil
}
