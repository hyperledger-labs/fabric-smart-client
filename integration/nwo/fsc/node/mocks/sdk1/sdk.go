/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk1

import "context"

type DummySDK struct {
}

func (d *DummySDK) Install(ctx context.Context) error {
	panic("implement me")
}

func (d *DummySDK) Start(ctx context.Context) error {
	panic("implement me")
}
