/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"

type KeyMapper[K any] func(K) (string, error)

func newEnhancedKVS[K any, V any](kvs *KVS, keyMapper KeyMapper[K]) *enhancedKVS[K, V] {
	return &enhancedKVS[K, V]{
		kvs:       kvs,
		keyMapper: keyMapper,
	}
}

type enhancedKVS[K any, V any] struct {
	kvs       *KVS
	keyMapper KeyMapper[K]
}

func (kvs *enhancedKVS[K, V]) Get(id K) (V, error) {
	k, err := kvs.keyMapper(id)
	if err != nil {
		return utils.Zero[V](), err
	}

	if !kvs.kvs.Exists(k) {
		return utils.Zero[V](), nil
	}
	var res V
	if err := kvs.kvs.Get(k, &res); err != nil {
		return utils.Zero[V](), err
	}
	return res, nil
}

func (kvs *enhancedKVS[K, V]) FilterExisting(inputKeys ...K) ([]K, error) {
	stringKeyMap := make(map[string]K, len(inputKeys))
	inputStrings := make([]string, len(inputKeys))
	for i, key := range inputKeys {
		k, err := kvs.keyMapper(key)
		if err != nil {
			return nil, err
		}
		inputStrings[i] = k
		stringKeyMap[k] = key
	}
	existingStrings := kvs.kvs.GetExisting(inputStrings...)
	existingKeys := make([]K, len(existingStrings))
	for i, key := range existingStrings {
		existingKeys[i] = stringKeyMap[key]
	}
	return existingKeys, nil
}

func (kvs *enhancedKVS[K, V]) Exists(id K) (bool, error) {
	k, err := kvs.keyMapper(id)
	if err != nil {
		return false, err
	}

	return kvs.kvs.Exists(k), nil
}

func (kvs *enhancedKVS[K, V]) Put(id K, info V) error {
	k, err := kvs.keyMapper(id)
	if err != nil {
		return err
	}
	return kvs.kvs.Put(k, info)
}
