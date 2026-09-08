/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk3

import "context"

type DummySDK struct{}

func (*DummySDK) Install() error {
	panic("implement me")
}

func (*DummySDK) Start(_ context.Context) error {
	panic("implement me")
}
