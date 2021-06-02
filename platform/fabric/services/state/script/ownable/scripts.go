/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package ownable

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/script/api"
)

const OwnableScript = "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/ownable"

func NewScripts() *api.Scripts {
	return &api.Scripts{
		Birth: &api.Script{
			Type: OwnableScript,
			Raw:  nil,
		},
		Death: &api.Script{
			Type: OwnableScript,
			Raw:  nil,
		},
	}
}

func IsScriptValid(s *api.Script) bool {
	return s != nil && s.Type == OwnableScript && len(s.Raw) == 0
}
