/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"crypto/tls"
	"fmt"
	"path"
	"runtime"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"golang.org/x/sync/errgroup"
	realgrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// grpcBufferSize is used to increase the grpc read and write buffer sizes from default 32k to 128k
const grpcBufferSize = 128 * 1024

type NamedFactory struct {
	Name    string
	Factory viewregistry.Factory
}

func GenerateConfig(testdataDir string) error {
	fscTopology := fsc.NewTopology()
	fscTopology.SetLogging("error", "")
	fscTopology.AddNodeByName("test-node")

	_, err := integration.GenerateAt(8099, testdataDir, false, fscTopology)
	if err != nil {
		return err
	}

	return nil
}

func SetupNode(confPath string, factories ...NamedFactory) (*node.Node, error) {
	fsc := node.NewWithConfPath(confPath)
	if err := fsc.InstallSDK(viewsdk.NewSDK(fsc)); err != nil {
		return nil, err
	}

	if err := fsc.Start(); err != nil {
		return nil, err
	}

	reg := viewregistry.GetRegistry(fsc)

	for _, f := range factories {
		err := reg.RegisterFactory(f.Name, f.Factory)
		if err != nil {
			return nil, err
		}
	}

	return fsc, nil
}

func SetupClient(confPath string) (*benchmark.ViewClient, func(), error) {
	config, err := view2.ConfigFromFile(confPath)
	if err != nil {
		return nil, nil, err
	}

	signer, err := client.NewX509SigningIdentity(config.SignerConfig.IdentityPath, config.SignerConfig.KeyPath)
	if err != nil {
		return nil, nil, err
	}

	signerIdentity, err := signer.Serialize()
	if err != nil {
		return nil, nil, err
	}

	cc := &grpc.ConnectionConfig{
		Address:           config.Address,
		TLSEnabled:        true,
		TLSRootCertFile:   path.Join(config.TLSConfig.PeerCACertPath),
		ConnectionTimeout: 10 * time.Second,
	}

	grpcClient, err := grpc.CreateGRPCClient(cc)
	if err != nil {
		return nil, nil, err
	}

	// note that we create the grpc connection here directly in order to set InsecureSkipVerify=true
	conn, err := realgrpc.NewClient(config.Address,
		realgrpc.WithTransportCredentials(
			credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}),
		),
		realgrpc.WithWriteBufferSize(grpcBufferSize),
		realgrpc.WithReadBufferSize(grpcBufferSize),
	)
	if err != nil {
		return nil, nil, err
	}

	tlsCert := grpcClient.Certificate()
	tlsCertHash, err := grpc.GetTLSCertHash(&tlsCert)
	if err != nil {
		return nil, nil, err
	}

	vc := &benchmark.ViewClient{
		SignF:       signer.Sign,
		Creator:     signerIdentity,
		TLSCertHash: tlsCertHash,
		Client:      protos.NewViewServiceClient(conn),
	}
	closeFunc := func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("failed to close connection: %v\n", err)
		}
	}

	return vc, closeFunc, nil
}

func WarmupClients(ccs []*benchmark.ViewClient, makeCaller func(*benchmark.ViewClient) func(ctx context.Context) error) error {
	g, ctx := errgroup.WithContext(context.Background())
	for _, cc := range ccs {
		caller := makeCaller(cc)
		for range runtime.NumCPU() {
			g.Go(func() error {
				for range 100 {
					err := caller(ctx)
					if err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
	return g.Wait()
}
