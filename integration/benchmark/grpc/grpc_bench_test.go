/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	benchviews "github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func BenchmarkGRPC(b *testing.B) {
	srvEndpoint := setupServer(b)

	// we share a single connection among all client goroutines
	cli, closeF := setupClient(b, srvEndpoint)
	defer closeF()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.CallViewWithContext(b.Context(), "fid", nil)
			require.NoError(b, err)
			require.NotNil(b, resp)
		}
	})
	benchmark.ReportTPS(b)
}

func setupServer(tb testing.TB) string {
	tb.Helper()

	mDefaultIdentity := view.Identity("server identity")
	mSigner := &benchmark.MockSigner{
		SerializeFunc: func() ([]byte, error) {
			return mDefaultIdentity.Bytes(), nil
		},
		SignFunc: func(bytes []byte) ([]byte, error) {
			return bytes, nil
		},
	}
	mIdentityProvider := &benchmark.MockIdentityProvider{DefaultSigner: mDefaultIdentity}
	mSigService := &benchmark.MockSignerProvider{DefaultSigner: mSigner}

	// marshaller
	tm, err := server.NewResponseMarshaler(mIdentityProvider, mSigService)
	require.NoError(tb, err)
	require.NotNil(tb, tm)

	// setup server
	grpcSrv, err := grpc.NewGRPCServer("localhost:0", grpc.ServerConfig{
		ConnectionTimeout: 0,
		SecOpts: grpc.SecureOptions{
			Certificate: certPEM,
			Key:         keyPEM,
			UseTLS:      true,
		},
		KaOpts:             grpc.KeepaliveOptions{},
		Logger:             nil,
		HealthCheckEnabled: false,
	})

	require.NoError(tb, err)
	require.NotNil(tb, grpcSrv)

	srv, err := server.NewViewServiceServer(tm, &server.YesPolicyChecker{}, server.NewMetrics(&disabled.Provider{}), noop.NewTracerProvider())
	require.NoError(tb, err)
	require.NotNil(tb, srv)

	parms := &benchviews.CPUParams{N: 200000}
	input, _ := json.Marshal(parms)
	factory := &benchviews.CPUViewFactory{}
	v, _ := factory.NewView(input)

	// our view manager
	vm := &benchmark.MockViewManager{Constructor: func() view.View {
		return v
	}}

	// register view manager wit grpc impl
	server.InstallViewHandler(vm, srv, noop.NewTracerProvider())

	// register grpc impl with grpc server
	protos.RegisterViewServiceServer(grpcSrv.Server(), srv)

	// start the actual grpc server
	go func() {
		_ = grpcSrv.Start()
	}()
	tb.Cleanup(grpcSrv.Stop)

	return grpcSrv.Address()
}

func setupClient(tb testing.TB, srvEndpoint string) (*benchmark.ViewClient, func()) {
	tb.Helper()

	mDefaultIdentity := view.Identity("client identity")
	mSigner := &benchmark.MockSigner{
		SerializeFunc: func() ([]byte, error) {
			return mDefaultIdentity.Bytes(), nil
		},
		SignFunc: func(bytes []byte) ([]byte, error) {
			return bytes, nil
		}}

	signerIdentity, err := mSigner.Serialize()
	require.NoError(tb, err)

	grpcClient, err := grpc.NewGRPCClient(grpc.ClientConfig{
		SecOpts: grpc.SecureOptions{
			ServerRootCAs: [][]byte{certPEM},
			UseTLS:        true,
		},
		KaOpts:       grpc.KeepaliveOptions{},
		Timeout:      5 * time.Second,
		AsyncConnect: false,
	})
	require.NoError(tb, err)
	require.NotNil(tb, grpcClient)

	conn, err := grpcClient.NewConnection(srvEndpoint)
	require.NoError(tb, err)
	require.NotNil(tb, conn)

	tlsCert := grpcClient.Certificate()
	tlsCertHash, err := grpc.GetTLSCertHash(&tlsCert)
	require.NoError(tb, err)

	cli := &benchmark.ViewClient{
		SignF:       mSigner.Sign,
		Creator:     signerIdentity,
		TLSCertHash: tlsCertHash,
		Client:      protos.NewViewServiceClient(conn),
	}

	return cli, func() {
		assert.NoError(tb, conn.Close())
	}
}
