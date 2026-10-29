/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package queryservice_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	cmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate/connstatetest"
)

// statusServer answers every transaction status query with a fixed status, so a test can
// tell which server served a query.
type statusServer struct {
	committerpb.UnimplementedQueryServiceServer
	status committerpb.Status
}

func (s *statusServer) GetTransactionStatus(context.Context, *committerpb.TxStatusQuery) (*committerpb.TxStatusResponse, error) {
	return &committerpb.TxStatusResponse{Statuses: []*committerpb.TxStatus{{Status: s.status}}}, nil
}

// startQueryServer starts an in-process query service on a free port and returns its
// address and the server, so a test can stop it early. It is stopped when the test ends.
func startQueryServer(t *testing.T, st committerpb.Status) (string, *grpc.Server) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	committerpb.RegisterQueryServiceServer(srv, &statusServer{status: st})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), srv
}

// TestQueryService_FailsOverWhenConnectedEndpointStops covers an endpoint that goes down
// after the client connected to it: its connection reports IDLE rather than
// TRANSIENT_FAILURE, so it is still picked, and the query must be retried on the next one.
// The connection state follows the endpoint: A's series ends disconnected and B gets its own.
func TestQueryService_FailsOverWhenConnectedEndpointStops(t *testing.T) {
	t.Parallel()
	addrA, srvA := startQueryServer(t, committerpb.Status_COMMITTED)
	addrB, _ := startQueryServer(t, committerpb.Status_ABORTED_MVCC_CONFLICT)

	fakeConfigProvider := &cmock.ServiceConfigProvider{}
	fakeConfigProvider.QueryServiceConfigStub = func(string) (*config.Config, error) {
		return &config.Config{Endpoints: []commongrpc.ConnectionConfig{{Address: addrA}, {Address: addrB}}}, nil
	}
	cp := config.NewClientProvider(fakeConfigProvider)
	m, rec := connstatetest.NewMetrics()
	qs := queryservice.NewRemoteQueryService(&config.Config{RequestTimeout: 5 * time.Second}, func() (committerpb.QueryServiceClient, string, error) {
		cc, err := cp.QueryServiceClient("net")
		if err != nil {
			return nil, "", err
		}
		return committerpb.NewQueryServiceClient(cc), cc.Target(), nil
	}, func(endpoint string) *connstate.Tracker {
		return m.Tracker("net", "ch", connstate.ComponentFabricxQuery, endpoint)
	})

	st, err := qs.GetTransactionStatus("tx1")
	require.NoError(t, err)
	require.Equal(t, int32(committerpb.Status_COMMITTED), st, "served by A while it is up")

	srvA.Stop()
	cc, err := cp.QueryServiceClient("net")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return cc.GetState() != connectivity.Ready
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, addrA, cc.Target(), "the stopped endpoint is still picked: its connection is not failing yet")

	st, err = qs.GetTransactionStatus("tx1")
	require.NoError(t, err, "the query must be retried on the next endpoint")
	require.Equal(t, int32(committerpb.Status_ABORTED_MVCC_CONFLICT), st, "served by B")

	require.Equal(t, []connstate.State{connstate.Connected, connstate.Reconnecting, connstate.Disconnected}, rec.States(addrA), "the series of A must not stay connected after the failover")
	require.Equal(t, []connstate.State{connstate.Connected}, rec.States(addrB))
}
