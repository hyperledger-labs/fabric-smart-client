/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import "context"

type SDK struct{}

func NewSDK() *SDK {
	return &SDK{}
}

func (*SDK) Install() error {
	panic("implement me")
}

func (*SDK) Start(_ context.Context) error {
	panic("implement me")
}
