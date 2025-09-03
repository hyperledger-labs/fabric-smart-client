/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockKVS struct{}

func (k mockKVS) GetLongTerm(ctx context.Context, ephemeral view.Identity) (view.Identity, error) {
	return nil, nil
}
func (k mockKVS) HaveSameBinding(ctx context.Context, this, that view.Identity) (bool, error) {
	return false, nil
}
func (k mockKVS) PutBindings(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	return nil
}

type mockExtractor struct{}

func (m mockExtractor) ExtractPublicKey(id view.Identity) (any, error) {
	return []byte("id"), nil
}

func TestPKIResolveConcurrency(t *testing.T) {
	svc, err := endpoint.NewService(mockKVS{})
	require.NoError(t, err)

	ext := mockExtractor{}
	resolver := &endpoint.Resolver{}

	err = svc.AddPublicKeyExtractor(ext)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			svc.PkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	// setup
	bindingStore := &mock.BindingStore{}
	bindingStore.PutBindingsReturns(nil)

	service, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)
	ext := mockExtractor{}
	_, err = service.AddResolver(
		"alice",
		"fsc.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apple", "strawberry"},
		[]byte("alice_id"),
	)
	require.NoError(t, err)
	resolvers := service.Resolvers()
	assert.Len(t, resolvers, 1)
	assert.Equal(t, 0, bindingStore.PutBindingsCallCount())

	_, err = service.AddResolver(
		"alice",
		"fabric.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apricot"},
		[]byte("alice_id2"),
	)
	require.NoError(t, err)
	resolvers = service.Resolvers()
	assert.Len(t, resolvers, 1)
	assert.Equal(t, 1, bindingStore.PutBindingsCallCount())

	err = service.AddPublicKeyExtractor(ext)
	require.NoError(t, err)

	// found
	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"[::1]:1010",
		"alice_id",
	} {
		resultID, err := service.GetIdentity(label, []byte{})
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), []byte(resultID))

		resultID, _, _, err = service.Resolve(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), []byte(resultID))

		resolver, _, err := service.Resolver(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), resolver.ID)

		resolver, err = service.GetResolver(context.Background(), view.Identity(label))
		require.NoError(t, err)
		assert.Equal(t, []byte("alice_id"), resolver.ID)
	}

	// not found
	for _, label := range []string{
		"alice.fabric.domain",
		"pineapple",
		"bob",
		"apricot",
		"localhost:8080",
		"alice_id2",
	} {
		resultID, err := service.GetIdentity(label, []byte("no"))
		assert.Error(t, err)
		assert.ErrorIs(t, err, endpoint.ErrNotFound)
		assert.Equal(t, []byte(nil), []byte(resultID))

		_, _, _, err = service.Resolve(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, endpoint.ErrNotFound)

		_, _, err = service.Resolver(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, endpoint.ErrNotFound)

		_, err = service.GetResolver(context.Background(), view.Identity(label))
		require.Error(t, err)
		assert.ErrorIs(t, err, endpoint.ErrNotFound)
	}

	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"[::1]:1010",
		"alice_id",
	} {
		ok, err := service.RemoveResolver(view.Identity(label))
		require.NoError(t, err)
		assert.True(t, ok)

		resolvers = service.Resolvers()
		assert.Len(t, resolvers, 0)

		// add again
		_, err = service.AddResolver(
			"alice",
			"fsc.domain",
			map[string]string{string(endpoint.P2PPort): "localhost:1010"},
			[]string{"apple", "strawberry"},
			[]byte("alice_id"),
		)
		require.NoError(t, err)
	}

	// remove something that does not exist
	ok, err := service.RemoveResolver(view.Identity("bob"))
	require.Error(t, err)
	assert.ErrorIs(t, err, endpoint.ErrNotFound)
	assert.False(t, ok)
}

func TestPutBindings_MultipleEphemerals3(t *testing.T) {
	ctx := context.Background()

	// // Create mock DB and mock expectations
	_, mock1, err := sqlmock.New()
	// require.NoError(t, err)
	// defer func() { _ = db.Close() }()

	// // Wrap sqlmock's db into RWDB
	// rwdb := &common3.RWDB{
	// 	WriteDB: db,
	// 	ReadDB:  db,
	// }

	// // Prepare table names
	// tables := common2.TableNames{
	// 	Binding: "bindings",
	// }

	store := &mock.BindingStore{}

	// Create store using constructor
	// store, err := NewBindingStore(rwdb, tables)
	// require.NoError(t, err)

	// Input identities
	longTerm := view.Identity("long")
	e1 := view.Identity("eph1")
	e2 := view.Identity("eph2")

	// Expected SQL query

	expectedSQL := regexp.QuoteMeta(`SELECT long_term_id FROM bindings WHERE ephemeral_hash = $1`)
	mock1.ExpectQuery(expectedSQL).
		WithArgs(longTerm.UniqueID()).
		WillReturnRows(sqlmock.NewRows([]string{"long_term_id"})) // empty rows = no results

	expectedSQL = regexp.QuoteMeta(`INSERT INTO bindings (ephemeral_hash, long_term_id) VALUES ($1, $2), ($3, $4), ($5, $6) ON CONFLICT DO NOTHING;`)
	mock1.ExpectExec(expectedSQL).
		WithArgs(longTerm.UniqueID(), longTerm, e1.UniqueID(), longTerm, e2.UniqueID(), longTerm).
		WillReturnResult(sqlmock.NewResult(1, 2))

	err = store.PutBindings(ctx, longTerm, e1, e2)
	require.NoError(t, err)
	require.NoError(t, mock1.ExpectationsWereMet())
}
