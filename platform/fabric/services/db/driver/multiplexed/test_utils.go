/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
)

func MockTypeConfig[T any](typ driver.PersistenceType, config T) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(key string, val any) error {
		if strings.Contains(key, "type") {
			typPtr, ok := val.(*driver.PersistenceType)
			if !ok {
				return errors.Errorf("unexpected target type [%T]", val)
			}
			*typPtr = typ
		} else if strings.Contains(key, "opts") {
			optsPtr, ok := val.(*T)
			if !ok {
				return errors.Errorf("unexpected target type [%T]", val)
			}
			*optsPtr = config
		}
		return nil
	})
	return cp
}
