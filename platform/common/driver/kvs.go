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
	FilterExistingSigners(ids ...view.Identity) ([]view.Identity, error)
	PutSigner(id view.Identity) error
}

type AuditInfoStore interface {
	GetAuditInfo(ctx context.Context, id view.Identity) ([]byte, error)
	PutAuditInfo(ctx context.Context, id view.Identity, info []byte) error
}

type BindingStore interface {
	GetLongTerm(ephemeral view.Identity) (view.Identity, error)
	HaveSameBinding(this, that view.Identity) (bool, error)
	PutBinding(ephemeral, longTerm view.Identity) error
}

type MetadataStore[K any, M any] interface {
	GetMetadata(key K) (M, error)
	ExistMetadata(key K) (bool, error)
	PutMetadata(key K, transientMap M) error
}

type EnvelopeStore[K any] interface {
	GetEnvelope(key K) ([]byte, error)
	ExistsEnvelope(key K) (bool, error)
	PutEnvelope(key K, env []byte) error
}

type EndorseTxStore[K any] interface {
	GetEndorseTx(key K) ([]byte, error)
	ExistsEndorseTx(key K) (bool, error)
	PutEndorseTx(key K, etx []byte) error
}
