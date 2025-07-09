/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testing

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
)

func MockConfig[T any](config T) *common.Config {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, val interface{}) error {
		*val.(*T) = config
		return nil
	})
	return common.NewConfig(cp)
}
