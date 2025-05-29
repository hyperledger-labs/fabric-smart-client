/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger()

type mds struct {
	metadataKVS driver.MetadataStore
	key         func(driver2.TxID) driver.Key
}

func NewMetadataService(metadataKVS driver.MetadataStore, network string, channel string) *mds {
	return &mds{metadataKVS: metadataKVS, key: keyMapper(network, channel)}
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
	key         func(driver2.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network string, channel string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network, channel)}
}

func (s *envs) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.envelopeKVS.ExistsEnvelope(ctx, s.key(txid))
	return ok
}

func (s *envs) StoreEnvelope(ctx context.Context, txID string, env interface{}) error {
	switch e := env.(type) {
	case []byte:
		return s.envelopeKVS.PutEnvelope(ctx, s.key(txID), e)
	case *common.Envelope:
		envBytes, err := proto.Marshal(e)
		if err != nil {
			return errors.WithMessagef(err, "failed marshalling envelop for tx [%s]", txID)
		}
		return s.envelopeKVS.PutEnvelope(ctx, s.key(txID), envBytes)
	default:
		return errors.Errorf("invalid env, expected []byte or *common.Envelope, got [%T]", env)
	}
}

func (s *envs) LoadEnvelope(ctx context.Context, txid string) ([]byte, error) {
	return s.envelopeKVS.GetEnvelope(ctx, s.key(txid))
}

type ets struct {
	endorseTxKVS driver.EndorseTxStore
	key          func(driver2.TxID) driver.Key
}

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network string, channel string) *ets {
	return &ets{endorseTxKVS: endorseTxKVS, key: keyMapper(network, channel)}
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

func keyMapper(network, channel string) func(txID driver2.TxID) driver.Key {
	return func(txID driver2.TxID) driver.Key {
		return driver.Key{Network: network, Channel: channel, TxID: txID}
	}
}
