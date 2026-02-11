/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/grpc/remote/workload"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var workloadProcessors = map[string]workload.ServerFunc{
	workload.ECDSA: workload.ProcessECDSA,
	workload.ECHO:  workload.ProcessEcho,
	workload.CPU:   workload.ProcessCPU,
}

func main() {
	keyPEM, certPEM, err := makeSelfSignedCert()
	if err != nil {
		panic(err)
	}

	grpcSrv, err := grpc.NewGRPCServer("0.0.0.0:8099", grpc.ServerConfig{
		ConnectionTimeout: 0,
		SecOpts: grpc.SecureOptions{
			Certificate: certPEM,
			Key:         keyPEM,
			UseTLS:      true,
		},
		Logger:             nil,
		HealthCheckEnabled: false,
	})
	if err != nil {
		panic(err)
	}

	s := grpcSrv.Server()
	remote.RegisterBenchmarkServiceServer(s, &BenchmarkService{})
	reflection.Register(s)

	go func() {
		_ = grpcSrv.Start()
	}()

	fmt.Printf("Running server on %v\n", grpcSrv.Address())

	// Wait on OS terminate signal.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch

	grpcSrv.Stop()
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

type BenchmarkService struct {
	remote.UnimplementedBenchmarkServiceServer
}

func (b *BenchmarkService) Process(ctx context.Context, in *remote.Request) (*remote.Response, error) {
	p, ok := workloadProcessors[in.Workload]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workload: %v", in.Workload)
	}

	return p(ctx, in)
}
