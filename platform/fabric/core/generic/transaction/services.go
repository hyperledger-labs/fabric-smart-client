/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"encoding/hex"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

var logger = logging.MustGetLogger()

type mds struct {
	metadataKVS driver.MetadataStore
	key         func(driver2.TxID) driver.Key
}

func NewMetadataService(metadataKVS driver.MetadataStore, network, channel string) *mds {
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

// fieldMappingStoreKey encodes (ns, key, valueDigest) into the TxID slot of driver.Key
// so field-mapping entries share the metadata KVS without colliding with real txids.
// The leading NUL-delimited "fieldmap" prefix keeps them disjoint from txid-shaped keys.
func fieldMappingStoreKey(ns, key string, valueDigest []byte) string {
	return "\x00fieldmap\x00" + ns + "\x00" + key + "\x00" + hex.EncodeToString(valueDigest)
}

func (s *mds) PutFieldMapping(ctx context.Context, ns, key string, valueDigest []byte, mapping driver.TransientMap) error {
	return s.metadataKVS.PutMetadata(ctx, s.key(fieldMappingStoreKey(ns, key, valueDigest)), mapping)
}

func (s *mds) GetFieldMapping(ctx context.Context, ns, key string, valueDigest []byte) (driver.TransientMap, error) {
	// A miss is not silent: the store reads no row, gets back nil bytes, and fails
	// unmarshalling them ("unexpected end of JSON input"). Callers must treat an
	// error here as "no mapping" rather than a hard failure. LoadTransient behaves
	// identically on a miss.
	return s.metadataKVS.GetMetadata(ctx, s.key(fieldMappingStoreKey(ns, key, valueDigest)))
}

type envs struct {
	envelopeKVS driver.EnvelopeStore
	key         func(driver2.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network, channel string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network, channel)}
}

func (s *envs) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.envelopeKVS.ExistsEnvelope(ctx, s.key(txid))
	return ok
}

func (s *envs) StoreEnvelope(ctx context.Context, txID string, env any) error {
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

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network, channel string) *ets {
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
