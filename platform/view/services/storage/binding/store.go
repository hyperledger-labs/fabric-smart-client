/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binding

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
)

func NewDefaultStore(cp driver.Config, d multiplexed.Driver) (driver.BindingStore, error) {
	return d.NewBinding(common.GetPersistenceName(cp, "fsc.binding.persistence"), "default")
}
