/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

var logger = logging.MustGetLogger()

type mds struct {
	metadataKVS driver.MetadataStore
	key         func(cdriver.TxID) driver.Key
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
	key         func(cdriver.TxID) driver.Key
}

func NewEnvelopeService(envelopeKVS driver.EnvelopeStore, network, channel string) *envs {
	return &envs{envelopeKVS: envelopeKVS, key: keyMapper(network, channel)}
}

func (s *envs) Exists(ctx context.Context, txid string) (bool, error) {
	return s.envelopeKVS.ExistsEnvelope(ctx, s.key(txid))
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
	key          func(cdriver.TxID) driver.Key
}

func NewEndorseTransactionService(endorseTxKVS driver.EndorseTxStore, network, channel string) *ets {
	return &ets{endorseTxKVS: endorseTxKVS, key: keyMapper(network, channel)}
}

func (s *ets) Exists(ctx context.Context, txid string) bool {
	ok, _ := s.endorseTxKVS.ExistsEndorseTx(ctx, s.key(txid))
	return ok
}

// StoreTransaction stores raw, a serialized endorsed transaction, under txid.
// If txid already holds a record, it keeps the stored one and returns nil when
// both are endorsements of the same proposal with the same results (see
// [sameEndorsement]), and an error wrapping [dbdriver.ErrConflict] otherwise.
func (s *ets) StoreTransaction(ctx context.Context, txid string, raw []byte) error {
	err := s.endorseTxKVS.PutEndorseTx(ctx, s.key(txid), raw)
	if !errors.Is(err, dbdriver.ErrConflict) {
		return err
	}
	stored, getErr := s.endorseTxKVS.GetEndorseTx(ctx, s.key(txid))
	if getErr != nil {
		return errors.WithMessagef(getErr, "failed reading stored transaction [%s]", txid)
	}
	if sameEndorsement(stored, raw) {
		return nil
	}
	return err
}

// endorsement holds the fields of a serialized transaction that identify what
// was endorsed. Fabric and Fabric-x transactions both serialize them as JSON
// under these names; Fabric wraps the transaction in a [SerializedTransaction],
// whose Raw holds it. A record stored as a TransactionEnvelope is converted to
// this JSON first, so JSON and envelope records of one endorsement compare equal.
type endorsement struct {
	Raw             []byte
	TSignedProposal *struct {
		ProposalBytes []byte `json:"proposal_bytes"`
		Signature     []byte `json:"signature"`
	}
	TProposalResponses []struct{ Payload []byte }
}

// sameEndorsement reports whether a and b, two serialized transactions, carry
// the same signed proposal and responses that all endorse the same payload.
// The payload holds the proposal hash and the results, while the endorser
// signatures, which differ on every signing, sit outside it. The records may
// hold different numbers of responses, as when a node endorses the same
// transaction with a second identity. A record that does not decode, has no
// signed proposal or no response is never the same as another.
func sameEndorsement(a, b []byte) bool {
	ea, ok := decodeEndorsement(a)
	if !ok {
		return false
	}
	eb, ok := decodeEndorsement(b)
	if !ok ||
		!bytes.Equal(ea.TSignedProposal.ProposalBytes, eb.TSignedProposal.ProposalBytes) ||
		!bytes.Equal(ea.TSignedProposal.Signature, eb.TSignedProposal.Signature) {
		return false
	}
	payload := ea.TProposalResponses[0].Payload
	for _, r := range append(ea.TProposalResponses, eb.TProposalResponses...) {
		if !bytes.Equal(r.Payload, payload) {
			return false
		}
	}
	return true
}

func decodeEndorsement(raw []byte) (endorsement, bool) {
	var e endorsement
	raw, err := protos.JSONFor(raw, "")
	if err != nil {
		return e, false
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return e, false
	}
	if e.Raw != nil {
		inner := e.Raw
		e = endorsement{}
		if err := json.Unmarshal(inner, &e); err != nil {
			return e, false
		}
	}
	return e, e.TSignedProposal != nil && len(e.TSignedProposal.ProposalBytes) > 0 && len(e.TProposalResponses) > 0
}

func (s *ets) LoadTransaction(ctx context.Context, txid string) ([]byte, error) {
	return s.endorseTxKVS.GetEndorseTx(ctx, s.key(txid))
}

func keyMapper(network, channel string) func(txID cdriver.TxID) driver.Key {
	return func(txID cdriver.TxID) driver.Key {
		return driver.Key{Network: network, Channel: channel, TxID: txID}
	}
}
