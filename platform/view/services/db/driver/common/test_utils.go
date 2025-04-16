/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs/mock"

func MockConfig[T any](config T) *mock.ConfigProvider {
	cp := &mock.ConfigProvider{}
	cp.UnmarshalKeyCalls(func(s string, i interface{}) error {
		*i.(*T) = config
		return nil
	})
	return cp
}
