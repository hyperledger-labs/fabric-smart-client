/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"
)

func MockTypeConfig[T any](typ driver.PersistenceType, config T) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(key string, val interface{}) error {
		if strings.Contains(key, "type") {
			*val.(*driver.PersistenceType) = typ
		} else if strings.Contains(key, "opts") {
			*val.(*T) = config
		}
		return nil
	})
	return cp
}
