/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("fabric-sdk.core")

type mds struct {
	metadataKVS driver.MetadataStore
	key         func(driver2.TxID) driver.Key
}

func NewMetadataService(metadataKVS driver.MetadataStore, network string, channel string) *mds {
	return &mds{metadataKVS: metadataKVS, key: keyMapper(network, channel)}
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
	key         func(driver2.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network string, channel string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network, channel)}
}

func (s *envs) Exists(txid string) bool {
	ok, _ := s.envelopeKVS.ExistsEnvelope(s.key(txid))
	return ok
}

func (s *envs) StoreEnvelope(txID string, env interface{}) error {
	switch e := env.(type) {
	case []byte:
		return s.envelopeKVS.PutEnvelope(s.key(txID), e)
	case *common.Envelope:
		envBytes, err := proto.Marshal(e)
		if err != nil {
			return errors.WithMessagef(err, "failed marshalling envelop for tx [%s]", txID)
		}
		return s.envelopeKVS.PutEnvelope(s.key(txID), envBytes)
	default:
		return errors.Errorf("invalid env, expected []byte or *common.Envelope, got [%T]", env)
	}
}

func (s *envs) LoadEnvelope(txid string) ([]byte, error) {
	return s.envelopeKVS.GetEnvelope(s.key(txid))
}

type ets struct {
	endorseTxKVS driver.EndorseTxStore
	key          func(driver2.TxID) driver.Key
}

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network string, channel string) *ets {
	return &ets{endorseTxKVS: endorseTxKVS, key: keyMapper(network, channel)}
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

func keyMapper(network, channel string) func(txID driver2.TxID) driver.Key {
	return func(txID driver2.TxID) driver.Key {
		return driver.Key{Network: network, Channel: channel, TxID: txID}
	}
}
