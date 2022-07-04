/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import "context"

type ServiceWrapper struct {
	*KVS
}

func (a *ServiceWrapper) Start(ctx context.Context) error {
	// NO-OP
	return nil
}

func (a *ServiceWrapper) Stop() error {
	a.KVS.Stop()
	return nil
}
