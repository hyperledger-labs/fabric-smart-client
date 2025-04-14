/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type identifier interface {
	UniqueKey() string
}

func NewEnvelopeStore[K identifier](e driver.EnvelopePersistence) *envelopeStore[K] {
	return &envelopeStore[K]{e: e}
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
