/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
)

type identifier interface {
	UniqueKey() string
}

func NewMetadataStore[K identifier, M any](m driver.MetadataPersistence) *metadataStore[K, M] {
	return &metadataStore[K, M]{m: m}
}

type metadataStore[K identifier, M any] struct {
	m driver.MetadataPersistence
}

func (s *metadataStore[K, M]) GetMetadata(key K) (M, error) {
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
func (s *metadataStore[K, M]) ExistMetadata(key K) (bool, error) {
	return s.m.ExistMetadata(key.UniqueKey())
}
func (s *metadataStore[K, M]) PutMetadata(key K, m M) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return s.m.PutMetadata(key.UniqueKey(), data)
}
