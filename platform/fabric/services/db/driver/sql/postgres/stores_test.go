/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// keyDataStore is the get/exists/put surface the envelope, endorsement-transaction
// and metadata stores share under their own method names.
type keyDataStore struct {
	get    func(context.Context, string) ([]byte, error)
	exists func(context.Context, string) (bool, error)
	put    func(context.Context, string, []byte) error
}

// TestKeyDataStoresPostgres round-trips each key-data store through a schema created
// on a real Postgres: a put value is read back, an unknown key is absent, and a
// second put for the same key is not an error and keeps the first value.
func TestKeyDataStoresPostgres(t *testing.T) {
	t.Parallel()

	terminate, pgConnStr, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)

	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: pgConnStr}))
	dbProvider := postgres.NewDbProvider()

	for name, open := range map[string]func(*testing.T) keyDataStore{
		"Envelope": func(t *testing.T) keyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewEnvelopeStore)
			require.NoError(t, err)
			return keyDataStore{s.GetEnvelope, s.ExistsEnvelope, s.PutEnvelope}
		},
		"EndorseTx": func(t *testing.T) keyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewEndorseTxStore)
			require.NoError(t, err)
			return keyDataStore{s.GetEndorseTx, s.ExistsEndorseTx, s.PutEndorseTx}
		},
		"Metadata": func(t *testing.T) keyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewMetadataStore)
			require.NoError(t, err)
			return keyDataStore{s.GetMetadata, s.ExistMetadata, s.PutMetadata}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := open(t)
			ctx := t.Context()
			// Not valid UTF-8 and containing a NUL, so a column that is not BYTEA rejects it.
			first := []byte{0x00, 0xff, 'f'}

			require.NoError(t, s.put(ctx, "tx1", first))
			require.NoError(t, s.put(ctx, "tx1", []byte("second")), "putting an existing key is not an error")

			got, err := s.get(ctx, "tx1")
			require.NoError(t, err)
			assert.Equal(t, first, got, "the first put wins")

			exists, err := s.exists(ctx, "tx1")
			require.NoError(t, err)
			assert.True(t, exists)

			got, err = s.get(ctx, "unknown")
			require.NoError(t, err)
			assert.Empty(t, got, "an unknown key returns nothing")

			exists, err = s.exists(ctx, "unknown")
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}
}
