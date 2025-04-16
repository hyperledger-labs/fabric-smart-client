/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
)

type identifier interface {
	UniqueKey() string
}

func NewStore[K identifier](cp driver.Config, d multiplexed.Driver, params ...string) (*envelopeStore[K], error) {
	e, err := d.NewEnvelope(db.CreateTableName("env", params...), db.NewPrefixConfig(cp, "fsc.envelope.persistence"))
	if err != nil {
		return nil, err
	}
	return &envelopeStore[K]{e: e}, nil
}

type envelopeStore[K identifier] struct {
	e driver.EnvelopePersistence
}

func (s *envelopeStore[K]) GetEnvelope(key K) ([]byte, error) {
	return s.e.GetEnvelope(key.UniqueKey())
}

func (s *envelopeStore[K]) ExistsEnvelope(key K) (bool, error) {
	return s.e.ExistsEnvelope(key.UniqueKey())
}

func (s *envelopeStore[K]) PutEnvelope(key K, etx []byte) error {
	return s.e.PutEnvelope(key.UniqueKey(), etx)
}
