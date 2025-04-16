/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
)

type identifier interface {
	UniqueKey() string
}

func NewStore[K identifier](cp driver.Config, d multiplexed.Driver, params ...string) (*endorseTxStore[K], error) {
	e, err := d.NewEndorseTx(db.NewPrefixConfig(cp, "fsc.endorsetx.persistence"), params...)
	if err != nil {
		return nil, err
	}
	return &endorseTxStore[K]{e: e}, nil
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
