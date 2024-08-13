/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"encoding/base64"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

type Hashable []byte

func (id Hashable) Raw() []byte {
	if len(id) == 0 {
		return nil
	}
	return utils.MustGet(utils.SHA256(id))
}

func (id Hashable) String() string { return base64.StdEncoding.EncodeToString(id.Raw()) }

func (id Hashable) RawString() string { return string(id.Raw()) }
