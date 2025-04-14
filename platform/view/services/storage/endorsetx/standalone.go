/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type identifier interface {
	UniqueKey() string
}

func NewEndorseTxStore[K identifier](e driver.EndorseTxPersistence) *endorseTxStore[K] {
	return &endorseTxStore[K]{e: e}
}

type endorseTxStore[K identifier] struct {
	e driver.EndorseTxPersistence
}

func (s *endorseTxStore[K]) GetEndorseTx(key K) ([]byte, error) {
	return s.e.GetEndorseTx(key.UniqueKey())
}
func (s *endorseTxStore[K]) ExistsEndorseTx(key K) (bool, error) {
	return s.e.ExistsEndorseTx(key.UniqueKey())
}
func (s *endorseTxStore[K]) PutEndorseTx(key K, etx []byte) error {
	return s.e.PutEndorseTx(key.UniqueKey(), etx)
}
