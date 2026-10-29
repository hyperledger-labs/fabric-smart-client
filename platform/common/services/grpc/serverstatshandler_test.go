/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc_test

//go:generate counterfeiter -o mock/counter.go -fake-name Counter github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics.Counter
//go:generate counterfeiter -o mock/gauge.go -fake-name Gauge github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics.Gauge
//go:generate counterfeiter -o mock/histogram.go -fake-name Histogram github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics.Histogram
//go:generate counterfeiter -o mock/provider.go -fake-name Provider github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics.Provider

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"

	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/testpb"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

func TestConnectionCounters(t *testing.T) {
	t.Parallel()

	openConn := &mock.Counter{}
	closedConn := &mock.Counter{}
	sh := &commongrpc.ServerStatsHandler{
		OpenConnCounter:   openConn,
		ClosedConnCounter: closedConn,
	}

	for i := 1; i <= 10; i++ {
		sh.HandleConn(t.Context(), &stats.ConnBegin{})
		require.Equal(t, i, openConn.AddCallCount())
	}

	for i := 1; i <= 5; i++ {
		sh.HandleConn(t.Context(), &stats.ConnEnd{})
		require.Equal(t, i, closedConn.AddCallCount())
	}
}

func TestConnMetricsGRPCServer(t *testing.T) {
	t.Parallel()

	openConn := &mock.Counter{}
	closedConn := &mock.Counter{}
	fakeProvider := &mock.Provider{}
	fakeProvider.NewCounterStub = func(o metrics.CounterOpts) metrics.Counter {
		switch o.Name {
		case "conn_opened":
			return openConn
		case "conn_closed":
			return closedConn
		default:
			panic("unknown counter")
		}
	}

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	srv, err := commongrpc.NewGRPCServerFromListener(
		listener,
		commongrpc.ServerConfig{
			SecOpts:            commongrpc.SecureOptions{UseTLS: false},
			ServerStatsHandler: commongrpc.NewServerStatsHandler(fakeProvider),
		},
	)
	require.NoError(t, err)

	// register the GRPC test server
	testpb.RegisterEmptyServiceServer(srv.Server(), &emptyServiceServer{})

	// start the server
	go func() { _ = srv.Start() }()
	defer srv.Stop()

	// test grpc connection counts
	require.Equal(t, 0, openConn.AddCallCount())
	require.Equal(t, 0, closedConn.AddCallCount())

	// create GRPC client conn
	var clientConns []*grpc.ClientConn
	for i := 1; i <= 3; i++ {
		clientConn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		clientConns = append(clientConns, clientConn)

		// invoke service
		client := testpb.NewEmptyServiceClient(clientConn)
		_, err = client.EmptyCall(t.Context(), &testpb.Empty{})
		require.NoError(t, err)
		require.Equal(t, i, openConn.AddCallCount())
	}

	for i, conn := range clientConns {
		require.Equal(t, i, closedConn.AddCallCount())
		_ = conn.Close()
		require.Eventually(t, func() bool { return closedConn.AddCallCount() == i+1 }, time.Second, 10*time.Millisecond)
	}
}
