/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"

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

func (s *mds) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.metadataKVS.ExistMetadata(ctx, s.key(txid))
	return ok
}

func (s *mds) StoreTransient(ctx context.Context, txid string, transientMap driver.TransientMap) error {
	return s.metadataKVS.PutMetadata(ctx, s.key(txid), transientMap)
}

func (s *mds) LoadTransient(ctx context.Context, txid string) (driver.TransientMap, error) {
	return s.metadataKVS.GetMetadata(ctx, s.key(txid))
}

type envs struct {
	envelopeKVS driver.EnvelopeStore
	key         func(id driver2.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network)}
}

func (s *envs) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.envelopeKVS.ExistsEnvelope(ctx, s.key(txid))
	return ok
}

func (s *envs) StoreEnvelope(ctx context.Context, txid string, env interface{}) error {
	switch e := env.(type) {
	case []byte:
		return s.envelopeKVS.PutEnvelope(ctx, s.key(txid), e)
	default:
		return errors.Errorf("invalid env, expected []byte, got [%T]", env)
	}
}

func (s *envs) LoadEnvelope(ctx context.Context, txid string) ([]byte, error) {
	return s.envelopeKVS.GetEnvelope(ctx, s.key(txid))
}

type ets struct {
	endorseTxKVS driver.EndorseTxStore
	key          func(id driver2.TxID) driver.Key
}

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network string) *ets {
	return &ets{endorseTxKVS: endorseTxKVS, key: keyMapper(network)}
}

func (s *ets) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.endorseTxKVS.ExistsEndorseTx(ctx, s.key(txid))
	return ok
}

func (s *ets) StoreTransaction(ctx context.Context, txid string, env []byte) error {
	return s.endorseTxKVS.PutEndorseTx(ctx, s.key(txid), env)
}

func (s *ets) LoadTransaction(ctx context.Context, txid string) ([]byte, error) {
	return s.endorseTxKVS.GetEndorseTx(ctx, s.key(txid))
}

func keyMapper(network string) func(txID driver2.TxID) driver.Key {
	return func(txID driver2.TxID) driver.Key { return driver.Key{Network: network, TxID: txID} }
}
