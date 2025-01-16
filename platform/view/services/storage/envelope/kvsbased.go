/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package envelope

import "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"

func NewKVSBased[K any](kvss *kvs.KVS, keyMapper kvs.KeyMapper[K]) *envelopeKVS[K] {
	return &envelopeKVS[K]{e: kvs.NewEnhancedKVS[K, []byte](kvss, keyMapper)}
}

type envelopeKVS[K any] struct {
	e *kvs.EnhancedKVS[K, []byte]
}

func (kvs *envelopeKVS[K]) GetEnvelope(key K) ([]byte, error)   { return kvs.e.Get(key) }
func (kvs *envelopeKVS[K]) ExistsEnvelope(key K) (bool, error)  { return kvs.e.Exists(key) }
func (kvs *envelopeKVS[K]) PutEnvelope(key K, env []byte) error { return kvs.e.Put(key, env) }
