/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ownable

import (
	state2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type State struct {
	state2.EmbeddingState
	State interface{}

	Owners []view.Identity
}

func (m *State) GetState() interface{} {
	return m.State
}
