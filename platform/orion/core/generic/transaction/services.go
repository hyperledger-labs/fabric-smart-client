/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger()

type mds struct {
	metadataKVS driver.MetadataStore
	key         func(id driver2.TxID) driver.Key
}

func NewMetadataService(metadataKVS driver.MetadataStore, network string) *mds {
	return &mds{metadataKVS: metadataKVS, key: keyMapper(network)}
}

func (s *mds) Exists(txid string) bool {
	ok, _ := s.metadataKVS.ExistMetadata(s.key(txid))
	return ok
}

func (s *mds) StoreTransient(txid string, transientMap driver.TransientMap) error {
	return s.metadataKVS.PutMetadata(s.key(txid), transientMap)
}

func (s *mds) LoadTransient(txid string) (driver.TransientMap, error) {
	return s.metadataKVS.GetMetadata(s.key(txid))
}

type envs struct {
	envelopeKVS driver.EnvelopeStore
	key         func(id driver2.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network)}
}

func (s *envs) Exists(txid string) bool {
	ok, _ := s.envelopeKVS.ExistsEnvelope(s.key(txid))
	return ok
}

func (s *envs) StoreEnvelope(txid string, env interface{}) error {
	switch e := env.(type) {
	case []byte:
		return s.envelopeKVS.PutEnvelope(s.key(txid), e)
	default:
		return errors.Errorf("invalid env, expected []byte, got [%T]", env)
	}
}

func (s *envs) LoadEnvelope(txid string) ([]byte, error) {
	return s.envelopeKVS.GetEnvelope(s.key(txid))
}

type ets struct {
	endorseTxKVS driver.EndorseTxStore
	key          func(id driver2.TxID) driver.Key
}

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network string) *ets {
	return &ets{endorseTxKVS: endorseTxKVS, key: keyMapper(network)}
}

func (s *ets) Exists(txid string) bool {
	ok, _ := s.endorseTxKVS.ExistsEndorseTx(s.key(txid))
	return ok
}

func (s *ets) StoreTransaction(txid string, env []byte) error {
	return s.endorseTxKVS.PutEndorseTx(s.key(txid), env)
}

func (s *ets) LoadTransaction(txid string) ([]byte, error) {
	return s.endorseTxKVS.GetEndorseTx(s.key(txid))
}

func keyMapper(network string) func(txID driver2.TxID) driver.Key {
	return func(txID driver2.TxID) driver.Key { return driver.Key{Network: network, TxID: txID} }
}
