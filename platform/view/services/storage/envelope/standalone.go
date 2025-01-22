/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

const (
	persistenceOptsConfigKey = "fsc.envelope.persistence.opts"
)

type identifier interface {
	UniqueKey() string
}

func NewWithConfig[K identifier](dbDriver driver.Driver, namespace string, cp db.Config) (driver2.EnvelopeStore[K], error) {
	e, err := dbDriver.NewEnvelope(namespace, db.NewPrefixConfig(cp, persistenceOptsConfigKey))
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
