/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("orion-sdk.core")

type mds struct {
	kvss    *kvs.KVS
	network string
}

func NewMetadataService(kvss *kvs.KVS, network string) *mds {
	return &mds{
		kvss:    kvss,
		network: network,
	}
}

func (s *mds) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return false
	}
	return s.kvss.Exists(key)
}

func (s *mds) StoreTransient(txid string, transientMap driver.TransientMap) error {
	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store transient for [%s]", txid)

	return s.kvss.Put(key, transientMap)
}

func (s *mds) LoadTransient(txid string) (driver.TransientMap, error) {
	logger.Debugf("load transient for [%s]", txid)

	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return nil, err
	}
	transientMap := driver.TransientMap{}
	err = s.kvss.Get(key, &transientMap)
	if err != nil {
		return nil, err
	}
	return transientMap, nil
}

type envs struct {
	kvss    *kvs.KVS
	network string
}

func NewEnvelopeService(kvss *kvs.KVS, network string) *envs {
	return &envs{
		kvss:    kvss,
		network: network,
	}
}

func (s *envs) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("envelope", []string{s.network, txid})
	if err != nil {
		return false
	}

	return s.kvss.Exists(key)
}

func (s *envs) StoreEnvelope(txid string, env interface{}) error {
	key, err := kvs.CreateCompositeKey("envelope", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store env for [%s]", txid)

	switch e := env.(type) {
	case []byte:
		return s.kvss.Put(key, e)
	default:
		return errors.Errorf("invalid env, expected []byte, got [%T]", env)
	}
}

func (s *envs) LoadEnvelope(txid string) ([]byte, error) {
	logger.Debugf("load env for [%s]", txid)

	key, err := kvs.CreateCompositeKey("envelope", []string{s.network, txid})
	if err != nil {
		return nil, err
	}
	env := []byte{}
	err = s.kvss.Get(key, &env)
	if err != nil {
		return nil, err
	}
	return env, nil
}

type ets struct {
	kvss    *kvs.KVS
	network string
}

func NewEndorseTransactionService(kvss *kvs.KVS, network string) *ets {
	return &ets{
		kvss:    kvss,
		network: network,
	}
}

func (s *ets) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return false
	}
	return s.kvss.Exists(key)
}

func (s *ets) StoreTransaction(txid string, env []byte) error {
	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store etx for [%s]", txid)

	return s.kvss.Put(key, env)
}

func (s *ets) LoadTransaction(txid string) ([]byte, error) {
	logger.Debugf("load etx for [%s]", txid)

	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return nil, err
	}
	env := []byte{}
	err = s.kvss.Get(key, &env)
	if err != nil {
		return nil, err
	}
	return env, nil
}
