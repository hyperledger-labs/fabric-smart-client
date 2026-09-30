/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package kvs_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections/iterators"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	kvs2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs/mock"
)

//go:generate counterfeiter -o mock/key_value_store.go -fake-name KeyValueStore github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver.KeyValueStore
//go:generate counterfeiter -o mock/config_provider.go -fake-name ConfigProvider . ConfigProvider

var sanitizer = postgres2.NewSanitizer()

type stuff struct {
	S string `json:"s"`
	I int    `json:"i"`
}

func testRound(t *testing.T, drv driver2.Driver) {
	t.Helper()
	kvsStore, err := drv.NewKVS("")
	require.NoError(t, err)
	kvstore, err := kvs2.New(kvsStore, "_default", kvs2.DefaultCacheSize)
	require.NoError(t, err)
	defer kvstore.Stop()

	k1, err := createCompositeKey("k", []string{"1"})
	require.NoError(t, err)
	k2, err := createCompositeKey("k", []string{"2"})
	require.NoError(t, err)

	err = kvstore.Put(context.Background(), k1, &stuff{"santa", 1})
	require.NoError(t, err)

	val := &stuff{}
	err = kvstore.Get(context.Background(), k1, val)
	require.NoError(t, err)
	require.Equal(t, &stuff{"santa", 1}, val)

	err = kvstore.Put(context.Background(), k2, &stuff{"claws", 2})
	require.NoError(t, err)

	val = &stuff{}
	err = kvstore.Get(context.Background(), k2, val)
	require.NoError(t, err)
	require.Equal(t, &stuff{"claws", 2}, val)

	it, err := kvstore.GetByPartialCompositeID(context.Background(), "k", []string{})
	require.NoError(t, err)
	defer func() { _ = it.Close() }()

	for ctr := 0; it.HasNext(); ctr++ {
		val = &stuff{}
		key, err := it.Next(val)
		require.NoError(t, err)
		switch ctr {
		case 0:
			require.Equal(t, k1, key)
			require.Equal(t, &stuff{"santa", 1}, val)
		case 1:
			require.Equal(t, k2, key)
			require.Equal(t, &stuff{"claws", 2}, val)
		default:
			require.Fail(t, "expected 2 entries in the range, found more")
		}
	}

	require.NoError(t, kvstore.Delete(context.Background(), k2))
	require.False(t, kvstore.Exists(context.Background(), k2))
	val = &stuff{}
	err = kvstore.Get(context.Background(), k2, val)
	require.Error(t, err)

	for ctr := 0; it.HasNext(); ctr++ {
		val = &stuff{}
		key, err := it.Next(val)
		require.NoError(t, err)
		if ctr == 0 {
			require.Equal(t, k1, key)
			require.Equal(t, &stuff{"santa", 1}, val)
		} else {
			require.Fail(t, "expected 2 entries in the range, found more")
		}
	}

	val = &stuff{
		S: "hello",
		I: 100,
	}
	h := sha256.Sum256([]byte("Hello World"))
	k, err := sanitizer.Encode(string(h[:]))
	require.NoError(t, err)
	require.NoError(t, kvstore.Put(context.Background(), k, val))
	require.True(t, kvstore.Exists(context.Background(), k))
	val2 := &stuff{}
	require.NoError(t, kvstore.Get(context.Background(), k, val2))
	require.Equal(t, val, val2)
	require.NoError(t, kvstore.Delete(context.Background(), k))
	require.False(t, kvstore.Exists(context.Background(), k))
}

func createCompositeKey(objectType string, attributes []string) (string, error) {
	k, err := kvs2.CreateCompositeKey(objectType, attributes)
	if err != nil {
		return "", err
	}
	return sanitizer.Encode(k)
}

func testParallelWrites(t *testing.T, drv driver2.Driver) {
	t.Helper()
	kvsStore, err := drv.NewKVS("")
	require.NoError(t, err)
	kvstore, err := kvs2.New(kvsStore, "_default", kvs2.DefaultCacheSize)
	require.NoError(t, err)
	defer kvstore.Stop()

	// different keys
	wg := sync.WaitGroup{}
	n := 100
	wg.Add(n)
	for i := range n {
		go func(i int) {
			k1, err := createCompositeKey("parallel_key_1_", []string{fmt.Sprintf("%d", i)})
			assert.NoError(t, err) // assert: in goroutine, require would only stop goroutine not test
			err = kvstore.Put(context.Background(), k1, &stuff{"santa", i})
			assert.NoError(t, err) // assert: in goroutine
			defer wg.Done()
		}(i)
	}
	wg.Wait()

	// same key
	wg = sync.WaitGroup{}
	wg.Add(n)
	k1, err := createCompositeKey("parallel_key_2_", []string{"1"})
	require.NoError(t, err)
	for i := range n {
		go func(_ int) {
			err := kvstore.Put(context.Background(), k1, &stuff{"santa", 1})
			assert.NoError(t, err) // assert: in goroutine
			defer wg.Done()
		}(i)
	}
	wg.Wait()
}

func TestMemoryKVS(t *testing.T) {
	t.Parallel()
	testRound(t, mem.NewDriver())
	testParallelWrites(t, mem.NewDriver())
}

func TestSQLiteKVS(t *testing.T) {
	t.Parallel()
	cp := multiplexed.MockTypeConfig(sqlite2.Persistence, sqlite2.Config{
		DataSource:      fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(t.TempDir(), "sqlite_test")),
		TablePrefix:     "",
		SkipCreateTable: false,
		SkipPragmas:     false,
		MaxOpenConns:    0,
		MaxIdleConns:    common.CopyPtr(2),
		MaxIdleTime:     common.CopyPtr(time.Minute),
	})
	testRound(t, sqlite2.NewDriver(cp))
	testParallelWrites(t, sqlite2.NewDriver(cp))
}

func TestPostgresKVS(t *testing.T) {
	t.Parallel()
	t.Log("starting postgres")
	terminate, pgConnStr, err := postgres2.StartPostgres(t.Context(), postgres2.ConfigFromEnv(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(terminate)
	t.Log("postgres ready")

	cp := multiplexed.MockTypeConfig(postgres2.Persistence, postgres2.Config{
		DataSource:      pgConnStr,
		TablePrefix:     "",
		SkipCreateTable: false,

		MaxOpenConns: 50,
		MaxIdleConns: common.CopyPtr(10),
		MaxIdleTime:  common.CopyPtr(time.Minute),
	})

	testRound(t, postgres2.NewDriver(cp))
	testParallelWrites(t, postgres2.NewDriver(cp))
}

// Unit Tests with Mocks

type testStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// readResult is a single (item, error) pair returned by resultIterator.Next.
type readResult struct {
	item *driver.UnversionedRead
	err  error
}

// resultIterator replays results in order and then reports exhaustion.
// It can yield errors, with or without an item.
type resultIterator struct {
	results []readResult
	nexts   int
	closed  bool
}

func (r *resultIterator) Next() (*driver.UnversionedRead, error) {
	r.nexts++
	if r.nexts > len(r.results) {
		return nil, nil
	}
	res := r.results[r.nexts-1]
	return res.item, res.err
}

func (r *resultIterator) Close() {
	r.closed = true
}

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		namespace string
		cacheSize int
		wantErr   bool
	}{
		{
			name:      "valid creation with default cache",
			namespace: "test_namespace",
			cacheSize: kvs2.DefaultCacheSize,
			wantErr:   false,
		},
		{
			name:      "valid creation with custom cache",
			namespace: "custom_namespace",
			cacheSize: 200,
			wantErr:   false,
		},
		{
			name:      "valid creation with zero cache",
			namespace: "zero_cache",
			cacheSize: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			k, err := kvs2.New(mockStore, tt.namespace, tt.cacheSize)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, k)
			} else {
				require.NoError(t, err)
				require.NotNil(t, k)
			}
		})
	}
}

func TestKVS_Put(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		id        string
		state     any
		setupMock func(*mock.KeyValueStore)
		wantErr   bool
		errMsg    string
	}{
		{
			name:  "successful put",
			id:    "key1",
			state: &testStruct{Name: "test", Value: 42},
			setupMock: func(m *mock.KeyValueStore) {
				m.SetStateReturns(nil)
			},
			wantErr: false,
		},
		{
			name:  "store error",
			id:    "key2",
			state: &testStruct{Name: "test", Value: 42},
			setupMock: func(m *mock.KeyValueStore) {
				m.SetStateReturns(fmt.Errorf("store error"))
			},
			wantErr: true,
			errMsg:  "store error",
		},
		{
			name:  "unmarshalable state",
			id:    "key3",
			state: make(chan int), // channels cannot be marshaled
			setupMock: func(_ *mock.KeyValueStore) {
				// SetState should not be called
			},
			wantErr: true,
			errMsg:  "cannot marshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			err = k.Put(context.Background(), tt.id, tt.state)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, mockStore.SetStateCallCount())
			}
		})
	}
}

func TestKVS_Get(t *testing.T) {
	t.Parallel()
	validJSON := []byte(`{"name":"test","value":42}`)

	tests := []struct {
		name      string
		id        string
		setupMock func(*mock.KeyValueStore)
		wantErr   bool
		errMsg    string
		expected  *testStruct
	}{
		{
			name: "successful get from store",
			id:   "key1",
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateReturns(validJSON, nil)
			},
			wantErr:  false,
			expected: &testStruct{Name: "test", Value: 42},
		},
		{
			name: "key not found",
			id:   "nonexistent",
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateReturns([]byte{}, nil)
			},
			wantErr: true,
			errMsg:  "does not exist",
		},
		{
			name: "store error",
			id:   "key2",
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateReturns(nil, fmt.Errorf("store error"))
			},
			wantErr: true,
			errMsg:  "failed retrieving state",
		},
		{
			name: "invalid json",
			id:   "key3",
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateReturns([]byte("invalid json"), nil)
			},
			wantErr: true,
			errMsg:  "cannot unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			result := &testStruct{}
			err = k.Get(context.Background(), tt.id, result)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestKVS_Get_CacheBehavior(t *testing.T) {
	t.Parallel()
	validJSON := []byte(`{"name":"cached","value":100}`)

	t.Run("cache populated by Put, used by Get", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		mockStore.SetStateReturns(nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		// Put value - populates cache
		err = k.Put(context.Background(), "key1", &testStruct{Name: "cached", Value: 100})
		require.NoError(t, err)
		require.Equal(t, 1, mockStore.SetStateCallCount())

		// Get should use cache (no GetState call)
		result := &testStruct{}
		err = k.Get(context.Background(), "key1", result)
		require.NoError(t, err)
		require.Equal(t, 0, mockStore.GetStateCallCount())
		require.Equal(t, "cached", result.Name)
		require.Equal(t, 100, result.Value)
	})

	t.Run("Get populates cache on miss (read-through)", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		mockStore.GetStateReturns(validJSON, nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		// First Get - cache miss, hits store and populates cache
		result1 := &testStruct{}
		err = k.Get(context.Background(), "key1", result1)
		require.NoError(t, err)
		require.Equal(t, 1, mockStore.GetStateCallCount())
		require.Equal(t, "cached", result1.Name)
		require.Equal(t, 100, result1.Value)

		// Second Get - cache hit, does NOT hit store
		// This demonstrates read-through caching: Get now populates the cache
		result2 := &testStruct{}
		err = k.Get(context.Background(), "key1", result2)
		require.NoError(t, err)
		require.Equal(t, 1, mockStore.GetStateCallCount())
		require.Equal(t, result1, result2)
	})
}

func TestKVS_Delete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		id        string
		setupMock func(*mock.KeyValueStore)
		wantErr   bool
	}{
		{
			name: "successful delete",
			id:   "key1",
			setupMock: func(m *mock.KeyValueStore) {
				m.DeleteStateReturns(nil)
			},
			wantErr: false,
		},
		{
			name: "store error",
			id:   "key2",
			setupMock: func(m *mock.KeyValueStore) {
				m.DeleteStateReturns(fmt.Errorf("delete error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			err = k.Delete(context.Background(), tt.id)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, mockStore.DeleteStateCallCount())
			}
		})
	}
}

func TestKVS_Exists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		id        string
		setupMock func(*mock.KeyValueStore)
		expected  bool
	}{
		{
			name: "key exists",
			id:   "existing_key",
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "existing_key", Raw: []byte(`{"name":"test","value":1}`)},
				}
				m.GetStateSetIteratorReturns(iterators.Slice(items), nil)
			},
			expected: true,
		},
		{
			name: "key does not exist",
			id:   "nonexistent_key",
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateSetIteratorReturns(iterators.Slice([]*driver.UnversionedRead{}), nil)
			},
			expected: false,
		},
		{
			name: "key exists with empty value",
			id:   "empty_key",
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "empty_key", Raw: []byte{}},
				}
				m.GetStateSetIteratorReturns(iterators.Slice(items), nil)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			result := k.Exists(context.Background(), tt.id)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestKVS_GetExisting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ids       []string
		setupMock func(*mock.KeyValueStore)
		expected  []string
	}{
		{
			name: "all keys exist",
			ids:  []string{"key1", "key2", "key3"},
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "key1", Raw: []byte(`{"name":"test1","value":1}`)},
					{Key: "key2", Raw: []byte(`{"name":"test2","value":2}`)},
					{Key: "key3", Raw: []byte(`{"name":"test3","value":3}`)},
				}
				m.GetStateSetIteratorReturns(iterators.Slice(items), nil)
			},
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name: "some keys exist",
			ids:  []string{"key1", "key2", "key3"},
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "key1", Raw: []byte(`{"name":"test1","value":1}`)},
					{Key: "key3", Raw: []byte(`{"name":"test3","value":3}`)},
				}
				m.GetStateSetIteratorReturns(iterators.Slice(items), nil)
			},
			expected: []string{"key1", "key3"},
		},
		{
			name: "no keys exist",
			ids:  []string{"key1", "key2"},
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateSetIteratorReturns(iterators.Slice([]*driver.UnversionedRead{}), nil)
			},
			expected: []string{},
		},
		{
			name: "empty value should not be included",
			ids:  []string{"key1", "key2"},
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "key1", Raw: []byte(`{"name":"test1","value":1}`)},
					{Key: "key2", Raw: []byte{}},
				}
				m.GetStateSetIteratorReturns(iterators.Slice(items), nil)
			},
			expected: []string{"key1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			result := k.GetExisting(context.Background(), tt.ids...)
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestKVS_GetExisting_Iterator(t *testing.T) {
	t.Parallel()
	errScan := fmt.Errorf("scan error")
	tests := []struct {
		name        string
		results     []readResult
		expected    []string
		wantNexts   int
		wantRefetch []string
	}{
		{
			name:        "no error",
			results:     []readResult{{item: &driver.UnversionedRead{Key: "key1", Raw: []byte("v1")}}},
			expected:    []string{"key1"},
			wantNexts:   2,
			wantRefetch: []string{"key2"},
		},
		{
			name:        "nil item with error",
			results:     []readResult{{err: errScan}},
			expected:    []string{},
			wantNexts:   1,
			wantRefetch: []string{"key1", "key2"},
		},
		{
			name: "non-nil item with error",
			results: []readResult{
				{item: &driver.UnversionedRead{Key: "key1", Raw: []byte("v1")}, err: errScan},
				{item: &driver.UnversionedRead{Key: "key2", Raw: []byte("v2")}},
			},
			expected:    []string{},
			wantNexts:   1,
			wantRefetch: []string{"key1", "key2"},
		},
		{
			name: "error after some items",
			results: []readResult{
				{item: &driver.UnversionedRead{Key: "key1", Raw: []byte("v1")}},
				{err: errScan},
				{item: &driver.UnversionedRead{Key: "key2", Raw: []byte("v2")}},
			},
			expected:    []string{"key1"},
			wantNexts:   2,
			wantRefetch: []string{"key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			it := &resultIterator{results: tt.results}
			mockStore := &mock.KeyValueStore{}
			mockStore.GetStateSetIteratorReturns(it, nil)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			require.ElementsMatch(t, tt.expected, k.GetExisting(context.Background(), "key1", "key2"))
			require.Equal(t, tt.wantNexts, it.nexts, "iteration must stop at exhaustion or the first error")
			require.True(t, it.closed, "iterator must be closed")

			// keys read before the error are cached; the others are fetched again
			k.GetExisting(context.Background(), "key1", "key2")
			require.Equal(t, 2, mockStore.GetStateSetIteratorCallCount())
			_, _, refetched := mockStore.GetStateSetIteratorArgsForCall(1)
			require.ElementsMatch(t, tt.wantRefetch, refetched)
		})
	}
}

func TestKVS_GetByPartialCompositeID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		prefix    string
		attrs     []string
		setupMock func(*mock.KeyValueStore)
		wantErr   bool
		errMsg    string
	}{
		{
			name:   "successful query",
			prefix: "prefix",
			attrs:  []string{"attr1", "attr2"},
			setupMock: func(m *mock.KeyValueStore) {
				items := []*driver.UnversionedRead{
					{Key: "prefix\x00attr1\x00attr2\x00key1", Raw: []byte(`{"name":"test1","value":1}`)},
					{Key: "prefix\x00attr1\x00attr2\x00key2", Raw: []byte(`{"name":"test2","value":2}`)},
				}
				m.GetStateRangeScanIteratorReturns(iterators.Slice(items), nil)
			},
			wantErr: false,
		},
		{
			name:   "store error",
			prefix: "prefix",
			attrs:  []string{"attr1"},
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateRangeScanIteratorReturns(nil, fmt.Errorf("store error"))
			},
			wantErr: true,
			errMsg:  "store access failure",
		},
		{
			name:   "empty results",
			prefix: "prefix",
			attrs:  []string{},
			setupMock: func(m *mock.KeyValueStore) {
				m.GetStateRangeScanIteratorReturns(iterators.Slice([]*driver.UnversionedRead{}), nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockStore := &mock.KeyValueStore{}
			tt.setupMock(mockStore)

			k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
			require.NoError(t, err)

			iter, err := k.GetByPartialCompositeID(context.Background(), tt.prefix, tt.attrs)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					require.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, iter)
				if iter != nil {
					_ = iter.Close()
				}
			}
		})
	}
}

func TestKVS_Iterator(t *testing.T) {
	t.Parallel()
	t.Run("iterate through results", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		items := []*driver.UnversionedRead{
			{Key: "key1", Raw: []byte(`{"name":"test1","value":1}`)},
			{Key: "key2", Raw: []byte(`{"name":"test2","value":2}`)},
			{Key: "key3", Raw: []byte(`{"name":"test3","value":3}`)},
		}
		mockStore.GetStateRangeScanIteratorReturns(iterators.Slice(items), nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		iter, err := k.GetByPartialCompositeID(context.Background(), "prefix", []string{})
		require.NoError(t, err)
		defer func() {
			require.NoError(t, iter.Close())
		}()

		count := 0
		for iter.HasNext() {
			state := &testStruct{}
			key, err := iter.Next(state)
			require.NoError(t, err)
			require.NotEmpty(t, key)
			require.NotNil(t, state)
			count++
		}
		require.Equal(t, 3, count)
	})

	t.Run("close iterator", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		items := []*driver.UnversionedRead{
			{Key: "key1", Raw: []byte(`{"name":"test1","value":1}`)},
		}
		mockStore.GetStateRangeScanIteratorReturns(iterators.Slice(items), nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		iter, err := k.GetByPartialCompositeID(context.Background(), "prefix", []string{})
		require.NoError(t, err)

		err = iter.Close()
		require.NoError(t, err)
	})
}

// resultIterator replays a scripted sequence of (item, error) pairs and then
// reports exhaustion, so tests can inject read failures into a range scan.
func TestKVS_Iterator_Errors(t *testing.T) {
	t.Parallel()
	errScan := fmt.Errorf("scan error")
	newIter := func(t *testing.T, results ...readResult) (kvs2.Iterator, *resultIterator) {
		t.Helper()
		scan := &resultIterator{results: results}
		mockStore := &mock.KeyValueStore{}
		mockStore.GetStateRangeScanIteratorReturns(scan, nil)
		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)
		iter, err := k.GetByPartialCompositeID(context.Background(), "prefix", nil)
		require.NoError(t, err)
		return iter, scan
	}

	tests := []struct {
		name      string
		results   []readResult
		wantKeys  []string
		wantNexts int
	}{
		{
			name:      "nil item with error",
			results:   []readResult{{err: errScan}},
			wantNexts: 1,
		},
		{
			name: "non-nil item with error",
			results: []readResult{
				{item: &driver.UnversionedRead{Key: "key1", Raw: []byte(`"v1"`)}, err: errScan},
				{item: &driver.UnversionedRead{Key: "key2", Raw: []byte(`"v2"`)}},
			},
			wantNexts: 1,
		},
		{
			name: "error after some items",
			results: []readResult{
				{item: &driver.UnversionedRead{Key: "key1", Raw: []byte(`"v1"`)}},
				{err: errScan},
				{item: &driver.UnversionedRead{Key: "key2", Raw: []byte(`"v2"`)}},
			},
			wantKeys:  []string{"key1"},
			wantNexts: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			iter, scan := newIter(t, tt.results...)

			var keys []string
			var nextErr error
			for iter.HasNext() {
				var v string
				key, err := iter.Next(&v)
				if err != nil {
					nextErr = err
					break
				}
				keys = append(keys, key)
			}
			require.ErrorIs(t, nextErr, errScan, "the read failure must reach the caller through Next")
			require.Equal(t, tt.wantKeys, keys)
			require.False(t, iter.HasNext(), "iteration must end after a read failure")
			require.Equal(t, tt.wantNexts, scan.nexts, "the store must not be read after a failure")
		})
	}

	t.Run("Next without a current item", func(t *testing.T) {
		t.Parallel()
		iter, _ := newIter(t, readResult{item: &driver.UnversionedRead{Key: "key1", Raw: []byte(`"v1"`)}})
		var v string

		_, err := iter.Next(&v)
		require.Error(t, err, "Next before HasNext")

		require.True(t, iter.HasNext())
		key, err := iter.Next(&v)
		require.NoError(t, err)
		require.Equal(t, "key1", key)
		require.Equal(t, "v1", v)

		require.False(t, iter.HasNext())
		_, err = iter.Next(&v)
		require.Error(t, err, "Next after HasNext returned false")
	})

	t.Run("Next after the read failure was returned", func(t *testing.T) {
		t.Parallel()
		iter, _ := newIter(t, readResult{err: errScan})
		var v string

		require.True(t, iter.HasNext())
		_, err := iter.Next(&v)
		require.ErrorIs(t, err, errScan)
		_, err = iter.Next(&v)
		require.Error(t, err)
		require.NotErrorIs(t, err, errScan, "the read failure is reported once")
	})
}

func TestKVS_Stop(t *testing.T) {
	t.Parallel()
	t.Run("successful stop", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		mockStore.CloseReturns(nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		k.Stop()
		require.Equal(t, 1, mockStore.CloseCallCount())
	})

	t.Run("stop with error", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		mockStore.CloseReturns(fmt.Errorf("close error"))

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		// Should not panic even with error
		k.Stop()
		require.Equal(t, 1, mockStore.CloseCallCount())
	})
}

// Concurrent operations are thoroughly tested by testParallelWrites with real databases (100 goroutines)

func TestCacheSizeFromConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setupMock      func(*mock.ConfigProvider)
		expectedSize   int
		expectedErrMsg string
	}{
		{
			name: "cache size not set - use default",
			setupMock: func(m *mock.ConfigProvider) {
				m.IsSetReturns(false)
			},
			expectedSize: kvs2.DefaultCacheSize,
		},
		{
			name: "valid cache size",
			setupMock: func(m *mock.ConfigProvider) {
				m.IsSetReturns(true)
				m.GetIntReturns(200)
			},
			expectedSize: 200,
		},
		{
			name: "zero cache size - error",
			setupMock: func(m *mock.ConfigProvider) {
				m.IsSetReturns(true)
				m.GetIntReturns(0)
			},
			expectedSize:   kvs2.DefaultCacheSize,
			expectedErrMsg: "expect value >= 1, actual 0",
		},
		{
			name: "negative cache size - error",
			setupMock: func(m *mock.ConfigProvider) {
				m.IsSetReturns(true)
				m.GetIntReturns(-1)
			},
			expectedSize:   kvs2.DefaultCacheSize,
			expectedErrMsg: "expect value >= 1, actual -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockConfig := &mock.ConfigProvider{}
			tt.setupMock(mockConfig)

			size, err := kvs2.CacheSizeFromConfig(mockConfig)

			require.Equal(t, tt.expectedSize, size)
			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestKVS_CacheInvalidation(t *testing.T) {
	t.Parallel()
	t.Run("delete invalidates cache", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		validJSON := []byte(`{"name":"test","value":42}`)

		// First get returns value
		mockStore.GetStateReturnsOnCall(0, validJSON, nil)
		// After delete, second get returns empty
		mockStore.GetStateReturnsOnCall(1, []byte{}, nil)
		mockStore.DeleteStateReturns(nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		// First get - populates cache
		result1 := &testStruct{}
		err = k.Get(context.Background(), "key1", result1)
		require.NoError(t, err)
		require.Equal(t, "test", result1.Name)

		// Delete - should invalidate cache
		err = k.Delete(context.Background(), "key1")
		require.NoError(t, err)

		// Second get - should go to store (cache invalidated)
		result2 := &testStruct{}
		err = k.Get(context.Background(), "key1", result2)
		require.Error(t, err)
	})

	t.Run("put updates cache", func(t *testing.T) {
		t.Parallel()
		mockStore := &mock.KeyValueStore{}
		mockStore.SetStateReturns(nil)

		k, err := kvs2.New(mockStore, "test_ns", kvs2.DefaultCacheSize)
		require.NoError(t, err)

		// Put value
		err = k.Put(context.Background(), "key1", &testStruct{Name: "test", Value: 42})
		require.NoError(t, err)

		// Get should use cache (no GetState call)
		result := &testStruct{}
		err = k.Get(context.Background(), "key1", result)
		require.NoError(t, err)
		require.Equal(t, "test", result.Name)
		require.Equal(t, 42, result.Value)
		require.Equal(t, 0, mockStore.GetStateCallCount())
	})
}
