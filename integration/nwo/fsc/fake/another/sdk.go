/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package another

import "context"

type SDK struct{}

func NewSDK() *SDK {
	return &SDK{}
}

func (d *SDK) Install() error {
	panic("implement me")
}

func (d *SDK) Start(ctx context.Context) error {
	panic("implement me")
}
