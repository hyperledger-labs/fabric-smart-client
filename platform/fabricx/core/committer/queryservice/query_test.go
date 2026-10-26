/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice_test

import (
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate/connstatetest"
)

// To re-generate the mock/ run "go generate" directive
//go:generate counterfeiter -o mock/query_service_client.go github.com/hyperledger/fabric-x-common/api/committerpb.QueryServiceClient

func setupTest(tb testing.TB) (*queryservice.RemoteQueryService, *mock.FakeQueryServiceClient) {
	tb.Helper()

	c := &config.Config{
		Endpoints:      nil,
		RequestTimeout: 5 * time.Second,
	}

	client := &mock.FakeQueryServiceClient{}
	qs := queryservice.NewRemoteQueryService(c, func() (committerpb.QueryServiceClient, error) { return client, nil }, nil)

	return qs, client
}

func raw(u uint64) []byte {
	return protowire.AppendVarint(nil, u)
}

func TestQueryService(t *testing.T) {
	t.Parallel()
	t.Run("GetState happy path", func(t *testing.T) {
		t.Parallel()
		qs, fake := setupTest(t)

		table := []struct {
			ns       string
			key      string
			q        *committerpb.Rows
			expected *driver.VaultValue
		}{
			{
				ns:  "ns1",
				key: "key1",
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
			q        *committerpb.Rows
			expected *driver.VaultValue
		}{
			{"ns1", "key1", nil, nil},
			{"ns1", "key1", &committerpb.Rows{}, nil},
			{"ns1", "key1", &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{}}, nil},
			{"ns1", "key1", &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{{NsId: "ns1"}}}, nil},
			{"ns1", "key1", &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{{NsId: "ns1", Rows: []*committerpb.Row{}}}}, nil},
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
			q        *committerpb.Rows
			expected map[driver.Namespace]map[driver.PKey]driver.VaultValue
		}{
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1"}},
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
						Rows: []*committerpb.Row{
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
			q        *committerpb.Rows
			expected map[driver.Namespace]map[driver.PKey]driver.VaultValue
		}{
			{
				m:        map[driver.Namespace][]driver.PKey{"ns1": {"key1"}},
				q:        &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{{NsId: "ns1"}}},
				expected: map[driver.Namespace]map[driver.PKey]driver.VaultValue{"ns1": {}},
			},
			{
				m: map[driver.Namespace][]driver.PKey{"ns1": {"key1", "doesnotexist"}},
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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
				q: &committerpb.Rows{Namespaces: []*committerpb.RowsNamespace{
					{
						NsId: "ns1",
						Rows: []*committerpb.Row{
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

		t.Run("happy path", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			// return a response with one status
			fake.GetTransactionStatusReturns(&committerpb.TxStatusResponse{
				Statuses: []*committerpb.TxStatus{
					{
						Status: committerpb.Status_COMMITTED,
					},
				},
			}, nil)

			code, err := qs.GetTransactionStatus("tx1")
			require.NoError(t, err)
			require.Equal(t, int32(committerpb.Status_COMMITTED), code)
		})

		t.Run("client error", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expectedError := errors.New("some error")
			fake.GetTransactionStatusReturns(nil, expectedError)

			_, err := qs.GetTransactionStatus("tx2")
			require.ErrorIs(t, err, expectedError)
		})

		t.Run("no statuses", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			fake.GetTransactionStatusReturns(&committerpb.TxStatusResponse{Statuses: []*committerpb.TxStatus{}}, nil)

			_, err := qs.GetTransactionStatus("tx3")
			require.Error(t, err)
		})
	})

	// New tests for GetTransactionStatuses
	t.Run("GetTransactionStatuses", func(t *testing.T) {
		t.Parallel()

		t.Run("happy path keyed by tx id", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			// statuses returned out of order and without the unknown tx3
			fake.GetTransactionStatusReturns(&committerpb.TxStatusResponse{
				Statuses: []*committerpb.TxStatus{
					{Ref: &committerpb.TxRef{TxId: "tx2"}, Status: committerpb.Status_ABORTED_MVCC_CONFLICT},
					{Ref: &committerpb.TxRef{TxId: "tx1"}, Status: committerpb.Status_COMMITTED},
				},
			}, nil)

			statuses, err := qs.GetTransactionStatuses([]string{"tx1", "tx2", "tx3"})
			require.NoError(t, err)
			require.Equal(t, map[string]int32{
				"tx1": int32(committerpb.Status_COMMITTED),
				"tx2": int32(committerpb.Status_ABORTED_MVCC_CONFLICT),
			}, statuses)
			require.Equal(t, 1, fake.GetTransactionStatusCallCount())
			_, query, _ := fake.GetTransactionStatusArgsForCall(0)
			require.Equal(t, []string{"tx1", "tx2", "tx3"}, query.GetTxIds())
		})

		t.Run("duplicate input ids are collapsed", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			fake.GetTransactionStatusReturns(&committerpb.TxStatusResponse{
				Statuses: []*committerpb.TxStatus{
					{Ref: &committerpb.TxRef{TxId: "tx1"}, Status: committerpb.Status_COMMITTED},
				},
			}, nil)

			statuses, err := qs.GetTransactionStatuses([]string{"tx1", "tx2", "tx1", "tx2"})
			require.NoError(t, err)
			require.Equal(t, map[string]int32{"tx1": int32(committerpb.Status_COMMITTED)}, statuses)
			_, query, _ := fake.GetTransactionStatusArgsForCall(0)
			require.Equal(t, []string{"tx1", "tx2"}, query.GetTxIds())
		})

		t.Run("statuses without ref are dropped", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			fake.GetTransactionStatusReturns(&committerpb.TxStatusResponse{
				Statuses: []*committerpb.TxStatus{
					{Status: committerpb.Status_COMMITTED},
					{Ref: &committerpb.TxRef{TxId: "tx1"}, Status: committerpb.Status_COMMITTED},
				},
			}, nil)

			statuses, err := qs.GetTransactionStatuses([]string{"tx1", "tx2"})
			require.NoError(t, err)
			require.Equal(t, map[string]int32{"tx1": int32(committerpb.Status_COMMITTED)}, statuses)
		})

		t.Run("empty input does not query", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)

			statuses, err := qs.GetTransactionStatuses(nil)
			require.NoError(t, err)
			require.Empty(t, statuses)
			require.Equal(t, 0, fake.GetTransactionStatusCallCount())
		})

		t.Run("client error", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expectedError := errors.New("some error")
			fake.GetTransactionStatusReturns(nil, expectedError)

			_, err := qs.GetTransactionStatuses([]string{"tx1"})
			require.ErrorIs(t, err, expectedError)
		})
	})

	// New tests for GetConfigTransaction
	t.Run("GetConfigTransaction", func(t *testing.T) {
		t.Parallel()

		t.Run("happy path", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			// Create a valid protobuf envelope
			envelope := &cb.Envelope{
				Payload:   []byte("test-payload"),
				Signature: []byte("test-signature"),
			}
			envelopeBytes, err := proto.Marshal(envelope)
			require.NoError(t, err)

			version := uint64(12345)
			fake.GetConfigTransactionReturns(&applicationpb.ConfigTransaction{
				Envelope: envelopeBytes,
				Version:  version,
			}, nil)

			resp, err := qs.GetConfigTransaction()
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.NotNil(t, resp.Envelope)
			require.Equal(t, envelope.Payload, resp.Envelope.Payload)
			require.Equal(t, envelope.Signature, resp.Envelope.Signature)
			require.Equal(t, version, resp.Version)
		})

		t.Run("client error", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expectedError := errors.New("some error")
			fake.GetConfigTransactionReturns(nil, expectedError)

			_, err := qs.GetConfigTransaction()
			require.ErrorIs(t, err, expectedError)
		})
	})

	// New tests for GetNamespacePolicies
	t.Run("GetNamespacePolicies", func(t *testing.T) {
		t.Parallel()

		t.Run("happy path", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expected := &applicationpb.NamespacePolicies{
				Policies: []*applicationpb.PolicyItem{
					{Namespace: "ns1", Policy: []byte("policy1"), Version: 0},
					{Namespace: "ns2", Policy: []byte("policy2"), Version: 3},
				},
			}
			fake.GetNamespacePoliciesReturns(expected, nil)

			resp, err := qs.GetNamespacePolicies()
			require.NoError(t, err)
			require.Equal(t, expected, resp)
			require.Equal(t, 1, fake.GetNamespacePoliciesCallCount())
		})

		t.Run("empty policies", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expected := &applicationpb.NamespacePolicies{}
			fake.GetNamespacePoliciesReturns(expected, nil)

			resp, err := qs.GetNamespacePolicies()
			require.NoError(t, err)
			require.Equal(t, expected, resp)
			require.Empty(t, resp.GetPolicies())
		})

		t.Run("client error", func(t *testing.T) {
			t.Parallel()
			qs, fake := setupTest(t)
			expectedError := errors.New("some error")
			fake.GetNamespacePoliciesReturns(nil, expectedError)

			_, err := qs.GetNamespacePolicies()
			require.ErrorIs(t, err, expectedError)
		})
	})
}

// RemoteQueryService asks its client factory for a stub on every request, so consecutive
// requests can go to different endpoints, and a factory error fails only that request.
func TestQueryService_ClientPerRequest(t *testing.T) {
	t.Parallel()
	committed := &committerpb.TxStatusResponse{Statuses: []*committerpb.TxStatus{{Status: committerpb.Status_COMMITTED}}}

	t.Run("each request uses the client the factory returns", func(t *testing.T) {
		t.Parallel()
		clientA, clientB := &mock.FakeQueryServiceClient{}, &mock.FakeQueryServiceClient{}
		clientA.GetTransactionStatusReturns(committed, nil)
		clientB.GetTransactionStatusReturns(committed, nil)
		clients := []committerpb.QueryServiceClient{clientA, clientB}
		qs := queryservice.NewRemoteQueryService(&config.Config{RequestTimeout: 5 * time.Second}, func() (committerpb.QueryServiceClient, error) {
			next := clients[0]
			clients = clients[1:]
			return next, nil
		}, nil)

		_, err := qs.GetTransactionStatus("tx1")
		require.NoError(t, err)
		_, err = qs.GetTransactionStatus("tx2")
		require.NoError(t, err)

		require.Equal(t, 1, clientA.GetTransactionStatusCallCount())
		require.Equal(t, 1, clientB.GetTransactionStatusCallCount())
	})

	t.Run("factory error fails the request", func(t *testing.T) {
		t.Parallel()
		expectedError := errors.New("no endpoint available")
		qs := queryservice.NewRemoteQueryService(&config.Config{RequestTimeout: 5 * time.Second}, func() (committerpb.QueryServiceClient, error) {
			return nil, expectedError
		}, nil)

		_, err := qs.GetTransactionStatus("tx1")
		require.ErrorIs(t, err, expectedError)
		_, err = qs.GetState("ns", "key")
		require.ErrorIs(t, err, expectedError)
	})
}

func TestQueryService_ReportsConnectionState(t *testing.T) {
	t.Parallel()
	m, rec := connstatetest.NewMetrics()
	client := &mock.FakeQueryServiceClient{}
	qs := queryservice.NewRemoteQueryService(&config.Config{RequestTimeout: time.Second}, func() (committerpb.QueryServiceClient, error) { return client, nil }, func() *connstate.Tracker {
		return m.Tracker("net", "ch", connstate.ComponentFabricxQuery, "committer")
	})
	answer := &committerpb.TxStatusResponse{Statuses: []*committerpb.TxStatus{{}}}

	for _, call := range []struct {
		res *committerpb.TxStatusResponse
		err error
	}{
		{nil, status.Error(codes.Unavailable, "down")}, // never answered: no series yet
		{answer, nil}, // connected
		{nil, status.Error(codes.DeadlineExceeded, "slow")},       // unchanged
		{nil, status.Error(codes.Unavailable, "down")},            // reconnecting
		{nil, status.Error(codes.InvalidArgument, "bad request")}, // answered: connected
	} {
		client.GetTransactionStatusReturns(call.res, call.err)
		_, _ = qs.GetTransactionStatus("tx1")
	}

	require.Equal(t, []connstate.State{connstate.Connected, connstate.Reconnecting, connstate.Connected}, rec.States("committer"))
}
