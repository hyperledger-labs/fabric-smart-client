/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice_test

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault/queryservice/fakes"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-committer/api/protonotify"
	"github.com/hyperledger/fabric-x-committer/api/protoqueryservice"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func setupTest(tb testing.TB) (*queryservice.RemoteQueryService, *fakes.FakeQueryServiceClient) {
	tb.Helper()

	config := &queryservice.Config{
		Endpoints:    nil,
		QueryTimeout: 5 * time.Second,
	}

	client := &fakes.FakeQueryServiceClient{}
	qs := queryservice.NewRemoteQueryService(config, client)

	return qs, client
}

func raw(u uint64) []byte {
	return protowire.AppendVarint(nil, u)
}

func TestQueryService(t *testing.T) {
	t.Run("GetState happy path", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		table := []struct {
			ns       string
			key      string
			q        *protoqueryservice.Rows
			expected *driver.VaultValue
		}{
			{
				ns:  "ns1",
				key: "key1",
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: &driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
			},
			{
				ns:  "ns1",
				key: "key2",
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key2"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: &driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
			},
			{
				ns:  "ns1",
				key: "key2",
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key2"),
								Value:   []byte(""),
								Version: 1,
							},
						},
					},
				}},
				expected: &driver.VaultValue{Raw: []byte(""), Version: raw(1)},
			},
		}

		for _, tc := range table {
			fake.GetRowsReturns(tc.q, nil)
			resp, err := qs.GetState(tc.ns, tc.key)
			require.NoError(t, err)
			require.Equal(t, tc.expected, resp)
		}
	})

	t.Run("GetState does not exist", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		table := []struct {
			ns       string
			key      string
			q        *protoqueryservice.Rows
			expected *driver.VaultValue
		}{
			{"ns1", "key1", nil, nil},
			{"ns1", "key1", &protoqueryservice.Rows{}, nil},
			{"ns1", "key1", &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{}}, nil},
			{"ns1", "key1", &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{{NsId: "ns1"}}}, nil},
			{"ns1", "key1", &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{{NsId: "ns1", Rows: []*protoqueryservice.Row{}}}}, nil},
		}

		for _, tc := range table {
			fake.GetRowsReturns(tc.q, nil)
			resp, err := qs.GetState(tc.ns, tc.key)
			require.NoError(t, err)
			require.Equal(t, tc.expected, resp)
		}
	})

	t.Run("GetState invalid query inputs", func(t *testing.T) {
		t.Parallel()
		qs, _ := setupTest(t)

		table := []struct {
			ns  string
			key string
		}{
			{"", ""},
			{"", "key1"},
			{"ns1", ""},
		}

		for _, tc := range table {
			_, err := qs.GetState(tc.ns, tc.key)
			require.Error(t, err)
		}
	})

	t.Run("GetStates happy path", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		table := []struct {
			m        map[driver.Namespace][]driver.PKey
			q        *protoqueryservice.Rows
			expected map[driver.Namespace]map[driver.PKey]driver.VaultValue
		}{
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key1": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
					},
				},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1", "key2"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
							{
								Key:     []byte("key2"),
								Value:   []byte("hello2"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key1": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
						"key2": driver.VaultValue{Raw: []byte("hello2"), Version: raw(0)},
					},
				},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1", "key2"}, "ns2": {"key3"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
							{
								Key:     []byte("key2"),
								Value:   []byte("hello2"),
								Version: 0,
							},
						},
					},
					{
						NsId: "ns2",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key3"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key1": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
						"key2": driver.VaultValue{Raw: []byte("hello2"), Version: raw(0)},
					},
					"ns2": {
						"key3": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
					},
				},
			},
		}

		for _, tc := range table {
			fake.GetRowsReturns(tc.q, nil)
			resp, err := qs.GetStates(tc.m)
			require.NoError(t, err)
			require.Equal(t, tc.expected, resp)
		}
	})

	t.Run("GetStates some do not exist", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		table := []struct {
			m        map[driver.Namespace][]driver.PKey
			q        *protoqueryservice.Rows
			expected map[driver.Namespace]map[driver.PKey]driver.VaultValue
		}{
			{
				m:        map[driver.Namespace][]driver.PKey{"ns1": {"key1"}},
				q:        &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{{NsId: "ns1"}}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{"ns1": {}},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1", "doesnotexist"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key1": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
					},
				},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"doesnotexist", "key2"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key2"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key2": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
					},
				},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1"}, "nsdoesnotexist": {"key2"}},
				q: &protoqueryservice.Rows{Namespaces: []*protoqueryservice.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*protoqueryservice.Row{
							{
								Key:     []byte("key1"),
								Value:   []byte("hello"),
								Version: 0,
							},
						},
					},
				}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{
					"ns1": {
						"key1": driver.VaultValue{Raw: []byte("hello"), Version: raw(0)},
					},
				},
			},
		}

		for _, tc := range table {
			fake.GetRowsReturns(tc.q, nil)
			resp, err := qs.GetStates(tc.m)
			require.NoError(t, err)
			require.Equal(t, tc.expected, resp)
		}
	})

	t.Run("GetStates invalid query inputs", func(t *testing.T) {
		t.Parallel()
		qs, _ := setupTest(t)

		table := []struct {
			m map[driver.Namespace][]driver.PKey
		}{
			{nil},
			{map[driver.Namespace][]driver.PKey{}},
			{map[driver.Namespace][]driver.PKey{"": {""}}},
			{map[driver.Namespace][]driver.PKey{"": {"key1"}}},
			{map[driver.Namespace][]driver.PKey{"ns1": {}}},
			{map[driver.Namespace][]driver.PKey{"ns1": {""}}},
			{map[driver.Namespace][]driver.PKey{"ns1": {"key1", ""}}},
			{map[driver.Namespace][]driver.PKey{"ns1": {"key1", "key2", ""}}},
			{map[driver.Namespace][]driver.PKey{"ns1": {"", "key2", ""}}},
		}

		for _, tc := range table {
			_, err := qs.GetStates(tc.m)
			require.Error(t, err)
		}
	})

	t.Run("GetState/s client return error", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		expectedError := errors.New("some error")

		fake.GetRowsReturns(nil, expectedError)
		_, err := qs.GetState("ns", "key1")
		require.ErrorIs(t, err, expectedError)
	})

	// New tests for GetTransactionStatus
	t.Run("GetTransactionStatus", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		t.Run("happy path", func(t *testing.T) {
			t.Parallel()
			// return a response with one status
			fake.GetTransactionStatusReturns(&protoqueryservice.TxStatusResponse{
				Statuses: []*protonotify.TxStatusEvent{
					{
						StatusWithHeight: &protoblocktx.StatusWithHeight{Code: 7},
					},
				},
			}, nil)

			code, err := qs.GetTransactionStatus("tx1")
			require.NoError(t, err)
			require.Equal(t, int32(7), code)
		})

		t.Run("client error", func(t *testing.T) {
			t.Parallel()
			expectedError := errors.New("some error")
			fake.GetTransactionStatusReturns(nil, expectedError)

			_, err := qs.GetTransactionStatus("tx2")
			require.ErrorIs(t, err, expectedError)
		})

		t.Run("no statuses", func(t *testing.T) {
			t.Parallel()
			fake.GetTransactionStatusReturns(&protoqueryservice.TxStatusResponse{Statuses: []*protonotify.TxStatusEvent{}}, nil)

			_, err := qs.GetTransactionStatus("tx3")
			require.Error(t, err)
		})
	})
}
