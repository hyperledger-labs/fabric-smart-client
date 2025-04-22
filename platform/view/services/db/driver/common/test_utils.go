/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"

func MockConfig[T any](config T) *config {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(_ string, val interface{}) error {
		*val.(*T) = config
		return nil
	})
	return NewConfig(cp)
}
