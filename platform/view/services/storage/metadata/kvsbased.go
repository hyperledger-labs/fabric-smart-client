/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
)

func NewKVSBased[K any, M any](kvss *kvs.KVS, keyMapper kvs.KeyMapper[K]) *metadataKVS[K, M] {
	return &metadataKVS[K, M]{e: kvs.NewEnhancedKVS[K, M](kvss, keyMapper)}
}

type metadataKVS[K any, M any] struct {
	e *kvs.EnhancedKVS[K, M]
}

func (kvs *metadataKVS[K, M]) GetMetadata(key K) (M, error) {
	return kvs.e.Get(key)
}
func (kvs *metadataKVS[K, M]) ExistMetadata(key K) (bool, error) { return kvs.e.Exists(key) }
func (kvs *metadataKVS[K, M]) PutMetadata(key K, tm M) error {
	return kvs.e.Put(key, tm)
}
