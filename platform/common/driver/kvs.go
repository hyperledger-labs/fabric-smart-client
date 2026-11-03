/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type SignerEntry struct {
	Signer     Signer
	DebugStack []byte
}

type SignerInfoStore interface {
	FilterExistingSigners(ctx context.Context, ids ...view.Identity) ([]view.Identity, error)
	PutSigner(ctx context.Context, id view.Identity) error
}

// AuditInfoStore stores the audit info of identities. It is write-once:
// PutAuditInfo with the audit info already stored returns nil, and with
// different audit info returns an error wrapping
// github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.ErrConflict
// and keeps the stored value.
type AuditInfoStore interface {
	GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error)
	PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error
}

type BindingStore interface {
	GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error)
	HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error)
	PutBindings(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
	ReplaceBinding(ctx context.Context, longTerm, ephemeral view.Identity) error
}

// MetadataStore stores transaction metadata by key. The first value written
// under a key wins; a later PutMetadata returns nil and keeps it.
type MetadataStore[K any, M any] interface {
	GetMetadata(ctx context.Context, key K) (M, error)
	ExistMetadata(ctx context.Context, key K) (bool, error)
	PutMetadata(ctx context.Context, key K, transientMap M) error
}

// EnvelopeStore stores transaction envelopes by key. It is write-once:
// PutEnvelope with the bytes already stored returns nil, and with different
// bytes returns an error wrapping
// github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.ErrConflict
// and keeps the stored value.
type EnvelopeStore[K any] interface {
	GetEnvelope(ctx context.Context, key K) ([]byte, error)
	ExistsEnvelope(ctx context.Context, key K) (bool, error)
	PutEnvelope(ctx context.Context, key K, env []byte) error
}

// EndorseTxStore stores endorsed transactions by key. It is write-once with
// the same semantics as [EnvelopeStore]; a caller that may store a re-signed
// copy of the same transaction compares the two records on conflict.
type EndorseTxStore[K any] interface {
	GetEndorseTx(ctx context.Context, key K) ([]byte, error)
	ExistsEndorseTx(ctx context.Context, key K) (bool, error)
	PutEndorseTx(ctx context.Context, key K, etx []byte) error
}
