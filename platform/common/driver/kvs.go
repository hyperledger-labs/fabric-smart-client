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

type AuditInfoStore interface {
	GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error)
	PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error
}

type BindingStore interface {
	GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error)
	HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error)
	PutBindings(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
}

type MetadataStore[K any, M any] interface {
	GetMetadata(ctx context.Context, key K) (M, error)
	ExistMetadata(ctx context.Context, key K) (bool, error)
	PutMetadata(ctx context.Context, key K, transientMap M) error
}

type EnvelopeStore[K any] interface {
	GetEnvelope(ctx context.Context, key K) ([]byte, error)
	ExistsEnvelope(ctx context.Context, key K) (bool, error)
	PutEnvelope(ctx context.Context, key K, env []byte) error
}

type EndorseTxStore[K any] interface {
	GetEndorseTx(ctx context.Context, key K) ([]byte, error)
	ExistsEndorseTx(ctx context.Context, key K) (bool, error)
	PutEndorseTx(ctx context.Context, key K, etx []byte) error
}
