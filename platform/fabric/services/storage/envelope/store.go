/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

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

func NewStore[K identifier](cp driver.Config, d multiplexed.Driver, params ...string) (*envelopeStore[K], error) {
	e, err := d.NewEnvelope(common.GetPersistenceName(cp, "fsc.envelope.persistence"), params...)
	if err != nil {
		return nil, err
	}
	return &envelopeStore[K]{e: e}, nil
}

type envelopeStore[K identifier] struct {
	e driver2.EnvelopeStore
}

func (s *envelopeStore[K]) GetEnvelope(ctx context.Context, key K) ([]byte, error) {
	return s.e.GetEnvelope(ctx, key.UniqueKey())
}

func (s *envelopeStore[K]) ExistsEnvelope(ctx context.Context, key K) (bool, error) {
	return s.e.ExistsEnvelope(ctx, key.UniqueKey())
}

func (s *envelopeStore[K]) PutEnvelope(ctx context.Context, key K, etx []byte) error {
	return s.e.PutEnvelope(ctx, key.UniqueKey(), etx)
}
