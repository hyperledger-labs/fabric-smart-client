/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/multiplexed"
)

type identifier interface {
	UniqueKey() string
}

func NewStore[K identifier, M any](cp driver.Config, d multiplexed.Driver, params ...string) (*store[K, M], error) {
	m, err := d.NewMetadata(common.GetPersistenceName(cp, "fsc.metadata.persistence"), params...)
	if err != nil {
		return nil, err
	}
	return &store[K, M]{m: m}, nil
}

type store[K identifier, M any] struct {
	m driver.MetadataStore
}

func (s *store[K, M]) GetMetadata(key K) (M, error) {
	var m M
	data, err := s.m.GetMetadata(key.UniqueKey())
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	return m, nil
}

func (s *store[K, M]) ExistMetadata(key K) (bool, error) {
	return s.m.ExistMetadata(key.UniqueKey())
}

func (s *store[K, M]) PutMetadata(key K, m M) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.m.PutMetadata(key.UniqueKey(), data)
}
