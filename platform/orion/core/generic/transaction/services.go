/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("orion-sdk.core")

type mds struct {
	sp      view2.ServiceProvider
	network string
}

func NewMetadataService(sp view2.ServiceProvider, network string) *mds {
	return &mds{
		sp:      sp,
		network: network,
	}
}

func (s *mds) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return false
	}
	return kvs.GetService(s.sp).Exists(key)
}

func (s *mds) StoreTransient(txid string, transientMap driver.TransientMap) error {
	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store transient for [%s]", txid)

	return kvs.GetService(s.sp).Put(key, transientMap)
}

func (s *mds) LoadTransient(txid string) (driver.TransientMap, error) {
	logger.Debugf("load transient for [%s]", txid)

	key, err := kvs.CreateCompositeKey("metadata", []string{s.network, txid})
	if err != nil {
		return nil, err
	}
	transientMap := driver.TransientMap{}
	err = kvs.GetService(s.sp).Get(key, &transientMap)
	if err != nil {
		return nil, err
	}
	return transientMap, nil
}

type envs struct {
	sp      view2.ServiceProvider
	network string
}

func NewEnvelopeService(sp view2.ServiceProvider, network string) *envs {
	return &envs{
		sp:      sp,
		network: network,
	}
}

func (s *envs) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("envelope", []string{s.network, txid})
	if err != nil {
		return false
	}

	return kvs.GetService(s.sp).Exists(key)
}

func (s *envs) StoreEnvelope(txid string, env interface{}) error {
	key, err := kvs.CreateCompositeKey("envelope", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store env for [%s]", txid)

	switch e := env.(type) {
	case []byte:
		return kvs.GetService(s.sp).Put(key, e)
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
	err = kvs.GetService(s.sp).Get(key, &env)
	if err != nil {
		return nil, err
	}
	return env, nil
}

type ets struct {
	sp      view2.ServiceProvider
	network string
}

func NewEndorseTransactionService(sp view2.ServiceProvider, network string) *ets {
	return &ets{
		sp:      sp,
		network: network,
	}
}

func (s *ets) Exists(txid string) bool {
	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return false
	}
	return kvs.GetService(s.sp).Exists(key)
}

func (s *ets) StoreTransaction(txid string, env []byte) error {
	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return err
	}
	logger.Debugf("store etx for [%s]", txid)

	return kvs.GetService(s.sp).Put(key, env)
}

func (s *ets) LoadTransaction(txid string) ([]byte, error) {
	logger.Debugf("load etx for [%s]", txid)

	key, err := kvs.CreateCompositeKey("etx", []string{s.network, txid})
	if err != nil {
		return nil, err
	}
	env := []byte{}
	err = kvs.GetService(s.sp).Get(key, &env)
	if err != nil {
		return nil, err
	}
	return env, nil
}
