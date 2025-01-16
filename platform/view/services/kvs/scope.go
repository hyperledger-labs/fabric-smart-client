/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Signer

func NewSignerKVS(kvs *KVS) *signerKVS {
	return &signerKVS{e: newEnhancedKVS[view.Identity, *sig.SignerEntry](kvs, signerKey)}
}

func signerKey(id view.Identity) (string, error) {
	return CreateCompositeKey("sigService", []string{"signer", id.UniqueID()})
}

type signerKVS struct {
	e *enhancedKVS[view.Identity, *sig.SignerEntry]
}

func (kvs *signerKVS) GetSigner(id view.Identity) (*sig.SignerEntry, error) { return kvs.e.Get(id) }
func (kvs *signerKVS) FilterExistingSigners(ids ...view.Identity) ([]view.Identity, error) {
	return kvs.e.FilterExisting(ids...)
}
func (kvs *signerKVS) PutSigner(id view.Identity, entry *sig.SignerEntry) error {
	return kvs.e.Put(id, entry)
}

// Audit info

func NewAuditInfoKVS(kvs *KVS) *auditInfoKVS {
	return &auditInfoKVS{e: newEnhancedKVS[view.Identity, []byte](kvs, auditInfoKey)}
}

func auditInfoKey(id view.Identity) (string, error) {
	return CreateCompositeKey("fsc.platform.view.sig", []string{id.UniqueID()})
}

type auditInfoKVS struct {
	e *enhancedKVS[view.Identity, []byte]
}

func (kvs *auditInfoKVS) GetAuditInfo(id view.Identity) ([]byte, error) { return kvs.e.Get(id) }
func (kvs *auditInfoKVS) PutAuditInfo(id view.Identity, info []byte) error {
	return kvs.e.Put(id, info)
}

// Binding

func NewBindingKVS(kvs *KVS) *bindingKVS {
	return &bindingKVS{e: newEnhancedKVS[view.Identity, view.Identity](kvs, bindingKey)}
}

type bindingKVS struct {
	e *enhancedKVS[view.Identity, view.Identity]
}

func (kvs *bindingKVS) HaveSameBinding(this, that view.Identity) (bool, error) {
	thisBinding, err := kvs.e.Get(this)
	if err != nil {
		return false, err
	}
	thatBinding, err := kvs.e.Get(that)
	if err != nil {
		return false, err
	}
	return thisBinding.Equal(thatBinding), nil
}
func (kvs *bindingKVS) GetBinding(ephemeral view.Identity) (view.Identity, error) {
	return kvs.e.Get(ephemeral)
}
func (kvs *bindingKVS) PutBinding(ephemeral, longTerm view.Identity) error {
	return kvs.e.Put(ephemeral, longTerm)
}

func bindingKey(ephemeral view.Identity) (string, error) {
	return CreateCompositeKey("platform.fsc.endpoint.binding", []string{ephemeral.UniqueID()})
}

func NewMetadataKVS[K any, M any](kvss *KVS, keyMapper KeyMapper[K]) *metadataKVS[K, M] {
	return &metadataKVS[K, M]{e: newEnhancedKVS[K, M](kvss, keyMapper)}
}

type metadataKVS[K any, M any] struct {
	e *enhancedKVS[K, M]
}

func (kvs *metadataKVS[K, M]) GetMetadata(key K) (M, error) {
	return kvs.e.Get(key)
}
func (kvs *metadataKVS[K, M]) ExistMetadata(key K) (bool, error) { return kvs.e.Exists(key) }
func (kvs *metadataKVS[K, M]) PutMetadata(key K, tm M) error {
	return kvs.e.Put(key, tm)
}

func NewEnvelopeKVS[K any](kvss *KVS, keyMapper KeyMapper[K]) *envelopeKVS[K] {
	return &envelopeKVS[K]{e: newEnhancedKVS[K, []byte](kvss, keyMapper)}
}

type envelopeKVS[K any] struct {
	e *enhancedKVS[K, []byte]
}

func (kvs *envelopeKVS[K]) GetEnvelope(key K) ([]byte, error)   { return kvs.e.Get(key) }
func (kvs *envelopeKVS[K]) ExistsEnvelope(key K) (bool, error)  { return kvs.e.Exists(key) }
func (kvs *envelopeKVS[K]) PutEnvelope(key K, env []byte) error { return kvs.e.Put(key, env) }

func NewEndorseTxKVS[K any](kvss *KVS, keyMapper KeyMapper[K]) *endorseTxKVS[K] {
	return &endorseTxKVS[K]{e: newEnhancedKVS[K, []byte](kvss, keyMapper)}
}

type endorseTxKVS[K any] struct {
	e *enhancedKVS[K, []byte]
}

func (kvs *endorseTxKVS[K]) GetEndorseTx(key K) ([]byte, error)   { return kvs.e.Get(key) }
func (kvs *endorseTxKVS[K]) ExistsEndorseTx(key K) (bool, error)  { return kvs.e.Exists(key) }
func (kvs *endorseTxKVS[K]) PutEndorseTx(key K, etx []byte) error { return kvs.e.Put(key, etx) }
