/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package binding

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
)

// NewDefaultStore constructs a BindingStore backed by the persistence configured under
// "fsc.binding.persistence", resolved through d.
func NewDefaultStore(cp driver.Config, d multiplexed.Driver) (driver.BindingStore, error) {
	name, err := common.GetPersistenceName(cp, "fsc.binding.persistence")
	if err != nil {
		return nil, err
	}
	return d.NewBinding(name, "default")
}
