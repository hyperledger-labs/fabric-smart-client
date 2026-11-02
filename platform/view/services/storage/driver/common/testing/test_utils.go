/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package testing

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/mock"
)

// MockConfig returns a Config whose persistence opts, under any name, are config.
// Other keys, such as fsc.id, are unset.
func MockConfig[T any](config T) *common.Config {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(key string, val any) error {
		if !strings.HasSuffix(key, ".opts") {
			return nil
		}
		ptr, ok := val.(*T)
		if !ok {
			return errors.Errorf("unexpected target type [%T]", val)
		}
		*ptr = config
		return nil
	})
	return common.NewConfig(cp)
}
