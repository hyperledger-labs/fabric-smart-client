/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorsetx

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"

func NewKVSBased[K any](kvss *kvs.KVS, keyMapper kvs.KeyMapper[K]) *endorseTxKVS[K] {
	return &endorseTxKVS[K]{e: kvs.NewEnhancedKVS[K, []byte](kvss, keyMapper)}
}

type endorseTxKVS[K any] struct {
	e *kvs.EnhancedKVS[K, []byte]
}

func (kvs *endorseTxKVS[K]) GetEndorseTx(key K) ([]byte, error)   { return kvs.e.Get(key) }
func (kvs *endorseTxKVS[K]) ExistsEndorseTx(key K) (bool, error)  { return kvs.e.Exists(key) }
func (kvs *endorseTxKVS[K]) PutEndorseTx(key K, etx []byte) error { return kvs.e.Put(key, etx) }
