/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import "github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"

type IdentityManager struct {
	im driver.IdentityManager
}

func (im *IdentityManager) Me() string {
	return im.im.Me()
}
