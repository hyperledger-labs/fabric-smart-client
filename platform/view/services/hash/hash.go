/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package hash

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
)

func SHA256(raw []byte) ([]byte, error) {
	return utils.SHA256(raw)
}

func SHA256OrPanic(raw []byte) []byte {
	return utils.MustGet(utils.SHA256(raw))
}
