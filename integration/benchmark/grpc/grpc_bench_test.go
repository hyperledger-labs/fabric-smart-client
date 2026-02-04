/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	benchviews "github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	serverSignerID = "default-server-id"
	clientSignerID = "default-client-id"

	cpuBurnVal = 200000
)

func BenchmarkGRPCImpl(b *testing.B) {
	cPath, kPath := setupCrypto(b)

	var benchmarks = []struct {
		name          string
		serverOptions []ServerOption
		clientOptions []ClientOption
	}{
		{
			name:          "w=noop/grpcsigner=mock",
			serverOptions: []ServerOption{WithServerMockSigner(serverSignerID), WithNOOPWorkload()},
			clientOptions: []ClientOption{WithClientMockSigner(clientSignerID)},
		},
		{
			name:          "w=cpu/grpcsigner=mock",
			serverOptions: []ServerOption{WithServerMockSigner(serverSignerID), WithCPUWorkload(cpuBurnVal)},
			clientOptions: []ClientOption{WithClientMockSigner(clientSignerID)},
		},
		{
			name:          "w=ecdsa/grpcsigner=mock",
			serverOptions: []ServerOption{WithServerMockSigner(serverSignerID), WithECDSAWorkload()},
			clientOptions: []ClientOption{WithClientMockSigner(clientSignerID)},
		},
		{
			name:          "w=noop/grpcsigner=ecdsa",
			serverOptions: []ServerOption{WithServerECDSASigner(cPath, kPath), WithNOOPWorkload()},
			clientOptions: []ClientOption{WithClientECDSASigner(cPath, kPath)},
		},
		{
			name:          "w=cpu/grpcsigner=ecdsa",
			serverOptions: []ServerOption{WithServerECDSASigner(cPath, kPath), WithCPUWorkload(cpuBurnVal)},
			clientOptions: []ClientOption{WithClientECDSASigner(cPath, kPath)},
		},
		{
			name:          "w=ecdsa/grpcsigner=ecdsa",
			serverOptions: []ServerOption{WithServerECDSASigner(cPath, kPath), WithECDSAWorkload()},
			clientOptions: []ClientOption{WithClientECDSASigner(cPath, kPath)},
		},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			runGPRCImpl(b, bm.serverOptions, bm.clientOptions)
		})
	}
}

func runGPRCImpl(b *testing.B, srvOptions []ServerOption, clientOptions []ClientOption) {
	srvEndpoint := setupServer(b, srvOptions...)

	// we share a single connection among all client goroutines
	cli, closeF := setupClient(b, srvEndpoint, clientOptions...)
	defer closeF()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.CallViewWithContext(b.Context(), "fid", nil)
			require.NoError(b, err)
			require.NotNil(b, resp)
		}
	})
	benchmark.ReportTPS(b)
}

// setupCrypto generates certificates and writes them to temporary files.
// It returns the paths and a cleanup function to delete the files after the test.
func setupCrypto(tb testing.TB) (certPath, keyPath string) {
	// 1. Call your helper to get PEM bytes
	keyPEM, certPEM, err := makeSelfSignedCert()
	require.NoError(tb, err)

	// 2. Create Temp Directory
	tmpDir := tb.TempDir()

	certPath = filepath.Join(tmpDir, "cert.pem")
	keyPath = filepath.Join(tmpDir, "key.pem")

	// 3. Write the PEM bytes directly to files
	err = os.WriteFile(certPath, certPEM, 0644)
	require.NoError(tb, err)

	err = os.WriteFile(keyPath, keyPEM, 0600)
	require.NoError(tb, err)

	return certPath, keyPath
}

// --- Option Types ---

type serverConfig struct {
	workload   view.View
	signer     client.SigningIdentity
	idProvider *benchmark.MockIdentityProvider
}

type clientConfig struct {
	signer client.SigningIdentity
}

type ServerOption func(tb testing.TB, c *serverConfig)
type ClientOption func(tb testing.TB, c *clientConfig)

// --- Server Options ---

func WithNOOPWorkload() ServerOption {
	return func(tb testing.TB, c *serverConfig) {
		factory := &benchviews.NoopViewFactory{}
		v, _ := factory.NewView(nil)
		c.workload = v
	}
}

func WithCPUWorkload(n int) ServerOption {
	return func(tb testing.TB, c *serverConfig) {
		params := &benchviews.CPUParams{N: n}
		input, _ := json.Marshal(params)
		factory := &benchviews.CPUViewFactory{}
		v, err := factory.NewView(input)
		require.NoError(tb, err)
		c.workload = v
	}
}

func WithECDSAWorkload() ServerOption {
	return func(tb testing.TB, c *serverConfig) {
		params := &benchviews.ECDSASignParams{}
		input, _ := json.Marshal(params)
		factory := &benchviews.ECDSASignViewFactory{}
		v, err := factory.NewView(input)
		require.NoError(tb, err)
		c.workload = v
	}
}

func WithServerMockSigner(id string) ServerOption {
	return func(tb testing.TB, c *serverConfig) {
		mIdentity := view.Identity(id)
		c.signer = &benchmark.MockSigner{
			SerializeFunc: func() ([]byte, error) { return mIdentity.Bytes(), nil },
			SignFunc:      func(b []byte) ([]byte, error) { return b, nil },
		}
		c.idProvider = &benchmark.MockIdentityProvider{DefaultSigner: mIdentity}
	}
}

func WithServerECDSASigner(certPath, keyPath string) ServerOption {
	return func(tb testing.TB, c *serverConfig) {
		signer, err := client.NewX509SigningIdentity(certPath, keyPath)
		require.NoError(tb, err)
		c.signer = signer
		serialized, _ := signer.Serialize()
		c.idProvider = &benchmark.MockIdentityProvider{DefaultSigner: view.Identity(serialized)}
	}
}

// --- Client Options ---

func WithClientMockSigner(id string) ClientOption {
	return func(tb testing.TB, c *clientConfig) {
		mIdentity := view.Identity(id)
		c.signer = &benchmark.MockSigner{
			SerializeFunc: func() ([]byte, error) { return mIdentity.Bytes(), nil },
			SignFunc:      func(b []byte) ([]byte, error) { return b, nil },
		}
	}
}

func WithClientECDSASigner(certPath, keyPath string) ClientOption {
	return func(tb testing.TB, c *clientConfig) {
		signer, err := client.NewX509SigningIdentity(certPath, keyPath)
		require.NoError(tb, err)
		c.signer = signer
	}
}

// setupServer create a grpc server using the grpc server impl in `platform/view/services/view/grpc/server`
func setupServer(tb testing.TB, opts ...ServerOption) string {
	tb.Helper()

	cfg := &serverConfig{}
	// Apply options
	for _, opt := range opts {
		opt(tb, cfg)
	}

	// Validate that the options successfully populated the config
	require.NotNil(tb, cfg.workload, "server workload was not configured by options")
	require.NotNil(tb, cfg.signer, "server signer was not configured by options")
	require.NotNil(tb, cfg.idProvider, "server identity provider was not configured by options")

	mSigService := &benchmark.MockSignerProvider{DefaultSigner: cfg.signer}
	// marshaller
	tm, err := server.NewResponseMarshaler(cfg.idProvider, mSigService)
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
	// our view manager
	vm := &benchmark.MockViewManager{Constructor: func() view.View { return cfg.workload }}
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

// setupClient create a grpc client using the grpc client impl in `platform/view/services/view/grpc/client`
func setupClient(tb testing.TB, srvEndpoint string, opts ...ClientOption) (*benchmark.ViewClient, func()) {
	tb.Helper()

	cfg := &clientConfig{}
	// Apply options
	for _, opt := range opts {
		opt(tb, cfg)
	}
	// Validate that the signer is present before proceeding
	require.NotNil(tb, cfg.signer, "client signer was not configured by options")

	signerIdentity, err := cfg.signer.Serialize()
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
		SignF:       cfg.signer.Sign,
		Creator:     signerIdentity,
		TLSCertHash: tlsCertHash,
		Client:      protos.NewViewServiceClient(conn),
	}

	return cli, func() {
		assert.NoError(tb, conn.Close())
	}
}
