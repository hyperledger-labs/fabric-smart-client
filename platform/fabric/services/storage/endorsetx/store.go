/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"context"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
)

type identifier interface {
	UniqueKey() string
}

func NewStore[K identifier](cp driver.Config, d multiplexed.Driver, params ...string) (*endorseTxStore[K], error) {
	e, err := d.NewEndorseTx(common.GetPersistenceName(cp, "fsc.endorsetx.persistence"), params...)
	if err != nil {
		return nil, err
	}
	return &endorseTxStore[K]{e: e}, nil
}

type endorseTxStore[K identifier] struct {
	e driver2.EndorseTxStore
}

func (s *endorseTxStore[K]) GetEndorseTx(ctx context.Context, key K) ([]byte, error) {
	return s.e.GetEndorseTx(ctx, key.UniqueKey())
}

func (s *endorseTxStore[K]) ExistsEndorseTx(ctx context.Context, key K) (bool, error) {
	return s.e.ExistsEndorseTx(ctx, key.UniqueKey())
}

func (s *endorseTxStore[K]) PutEndorseTx(ctx context.Context, key K, etx []byte) error {
	return s.e.PutEndorseTx(ctx, key.UniqueKey(), etx)
}
