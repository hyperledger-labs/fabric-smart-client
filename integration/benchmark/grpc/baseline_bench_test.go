/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	benchviews "github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func BenchmarkGRPCBaseline(b *testing.B) {
	b.Run("w=noop", func(b *testing.B) { runGPRCBaseline(b, "noop") })
	b.Run("w=cpu", func(b *testing.B) { runGPRCBaseline(b, "cpu") })
	b.Run("w=ecdsa", func(b *testing.B) { runGPRCBaseline(b, "ecdsa") })
}

func runGPRCBaseline(b *testing.B, w string) {
	srvEndpoint := setupBaselineServer(b, w)

	// we share a single connection among all client goroutines
	client, closeF := setupBaselineClient(b, srvEndpoint)
	defer closeF()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := client.ProcessCommand(b.Context(), &protos.SignedCommand{})
			require.NoError(b, err)
			require.NotNil(b, resp)
		}
	})
	benchmark.ReportTPS(b)
}

var (
	keyPEM  []byte
	certPEM []byte
)

func init() {
	flogging.Init(flogging.Config{
		LogSpec: "grpc=error:error",
	})

	keyPEM, certPEM, _ = makeSelfSignedCert()
}

// makeSelfSignedCert generates a localhost self-signed cert using ECDSA P-256.
// It returns the tls.Certificate and the PEM-encoded cert for the client root pool.
func makeSelfSignedCert() ([]byte, []byte, error) {
	// 1. generate ECDSA private key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// 2. certificate template
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Local Test CA"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// 3. sign it (self-signed)
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	// 4. PEM-encode cert & private key
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})

	return keyPEM, certPEM, nil
}

type serverImpl struct {
	protos.UnimplementedViewServiceServer

	workload view.View
}

func (s *serverImpl) ProcessCommand(ctx context.Context, command *protos.SignedCommand) (*protos.SignedCommandResponse, error) {
	resp, err := s.workload.Call(nil)
	if err != nil {
		return nil, err
	}

	// TODO include resp in signed response
	_ = resp

	return &protos.SignedCommandResponse{}, nil
}

func (s *serverImpl) StreamCommand(g grpc.BidiStreamingServer[protos.SignedCommand, protos.SignedCommandResponse]) error {
	panic("Not needed")
}

// setupBaselineServer creates a grpc server registering serverImpl as RegisterViewServiceServer
func setupBaselineServer(tb testing.TB, workloadType string) string {
	tb.Helper()

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(tb, err)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	}

	// setup server
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(tb, err)

	var opts []grpc.ServerOption
	opts = append(opts, grpc.Creds(credentials.NewTLS(serverTLS)))
	grpcServer := grpc.NewServer(opts...)

	var v view.View
	switch workloadType {
	case "cpu":
		parms := &benchviews.CPUParams{N: 200000}
		input, _ := json.Marshal(parms)
		factory := &benchviews.CPUViewFactory{}
		v, _ = factory.NewView(input)
	case "ecdsa":
		parms := &benchviews.ECDSASignParams{}
		input, _ := json.Marshal(parms)
		factory := &benchviews.ECDSASignViewFactory{}
		v, _ = factory.NewView(input)
	case "noop":
		factory := &benchviews.NoopViewFactory{}
		v, _ = factory.NewView(nil)
	default:
		tb.Fatalf("unknown workload type: %s", workloadType)
	}

	srv := &serverImpl{workload: v}

	protos.RegisterViewServiceServer(grpcServer, srv)
	go func() {
		_ = grpcServer.Serve(lis)
	}()

	tb.Cleanup(func() {
		grpcServer.Stop()
	})

	return lis.Addr().String()
}

// setupBaselineClient creates a grpc client
func setupBaselineClient(tb testing.TB, srvEndpoint string) (protos.ViewServiceClient, func()) {
	tb.Helper()

	rootPool := x509.NewCertPool()
	ok := rootPool.AppendCertsFromPEM(certPEM)
	require.True(tb, ok)

	clientTLS := &tls.Config{
		RootCAs:    rootPool,
		ServerName: "localhost", // must match cert's DNSNames / SAN
	}

	var clientOpts []grpc.DialOption
	clientOpts = append(clientOpts, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))

	//conn, err := grpc.NewClient(srvEndpoint, clientOpts...)
	conn, err := grpc.NewClient(srvEndpoint,
		clientOpts...,
	)
	require.NoError(tb, err)

	return protos.NewViewServiceClient(conn), func() {
		assert.NoError(tb, conn.Close())
	}
}
