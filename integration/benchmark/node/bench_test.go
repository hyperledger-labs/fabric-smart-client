/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark/views"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/node"
	viewsdk "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	viewregistry "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client/cmd"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Benchmark(b *testing.B) {
	benchmarks := []struct {
		name    string
		factory viewregistry.Factory
		params  any
	}{
		{
			name:    "noop",
			factory: &views.NoopViewFactory{},
		},
		{
			name:    "cpu",
			factory: &views.CPUViewFactory{},
			params:  &views.CPUParams{N: 200000},
		},
		{
			name:    "sign",
			factory: &views.ECDSASignViewFactory{},
			params:  &views.ECDSASignParams{},
		},
	}

	testdataPath := b.TempDir() // for local debugging you can set testdataPath := "out/testdata"
	nodeConfPath := path.Join(testdataPath, "fsc", "nodes", "test-node.0")
	clientConfPath := path.Join(nodeConfPath, "client-config.yaml")

	// we generate our testdata
	err := generateConfig(b, testdataPath)
	require.NoError(b, err)

	// run all benchmarks via direct view API
	for _, bm := range benchmarks {
		b.Run(fmt.Sprintf("direct/%s", bm.name), func(b *testing.B) {
			n, err := setupNode(b, nodeConfPath, namedFactory{
				name:    bm.name,
				factory: bm.factory,
			})
			require.NoError(b, err)
			b.Cleanup(n.Stop)

			vm, err := viewregistry.GetManager(n)
			require.NoError(b, err)

			var in []byte
			if bm.params != nil {
				in, err = json.Marshal(bm.params)
				require.NoError(b, err)
			}

			b.RunParallel(func(pb *testing.PB) {
				// each goroutine instantiates a dedicated view
				f, err := vm.NewView(bm.name, in)
				assert.NoError(b, err)

				for pb.Next() {
					_, err = vm.InitiateView(f, context.Background())
					assert.NoError(b, err)
				}
			})
			benchmark.ReportTPS(b)
		})

		// run all benchmarks via grpc view API
		b.Run(fmt.Sprintf("grpc/%s", bm.name), func(b *testing.B) {
			n, err := setupNode(b, nodeConfPath, namedFactory{
				name:    bm.name,
				factory: bm.factory,
			})
			require.NoError(b, err)
			b.Cleanup(n.Stop)

			var in []byte
			if bm.params != nil {
				in, err = json.Marshal(bm.params)
				require.NoError(b, err)
			}

			// setup grpc client
			cli, err := setupClient(b, clientConfPath)
			require.NoError(b, err)

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, err := cli.CallViewWithContext(b.Context(), bm.name, in)
					assert.NoError(b, err)
				}
			})
			benchmark.ReportTPS(b)
		})
	}
}

func generateConfig(tb testing.TB, testdataDir string) error {
	tb.Helper()

	fscTopology := fsc.NewTopology()
	fscTopology.SetLogging("error", "")
	fscTopology.AddNodeByName("test-node")

	_, err := integration.GenerateAt(8000, testdataDir, false, fscTopology)
	if err != nil {
		return err
	}

	return nil
}

func setupNode(tb testing.TB, confPath string, factories ...namedFactory) (*node.Node, error) {
	tb.Helper()

	fsc := node.NewWithConfPath(confPath)
	if err := fsc.InstallSDK(viewsdk.NewSDK(fsc)); err != nil {
		return nil, err
	}

	if err := fsc.Start(); err != nil {
		return nil, err
	}

	reg := viewregistry.GetRegistry(fsc)

	for _, f := range factories {
		err := reg.RegisterFactory(f.name, f.factory)
		if err != nil {
			return nil, err
		}
	}

	return fsc, nil
}

type namedFactory struct {
	name    string
	factory viewregistry.Factory
}

func setupClient(tb testing.TB, confPath string) (*benchmark.ViewClient, error) {
	tb.Helper()

	config, err := view2.ConfigFromFile(confPath)
	if err != nil {
		return nil, err
	}

	signer, err := client.NewX509SigningIdentity(config.SignerConfig.IdentityPath, config.SignerConfig.KeyPath)
	if err != nil {
		return nil, err
	}

	signerIdentity, err := signer.Serialize()
	if err != nil {
		return nil, err
	}

	tlsEnabled := config.TLSConfig.Enabled
	tlsRootCACert := path.Clean(config.TLSConfig.RootCACertPath)

	cc := &grpc.ConnectionConfig{
		Address:           config.Address,
		TLSEnabled:        tlsEnabled,
		TLSRootCertFile:   tlsRootCACert,
		ConnectionTimeout: 10 * time.Second,
	}

	grpcClient, err := grpc.CreateGRPCClient(cc)
	if err != nil {
		return nil, err
	}

	conn, err := grpcClient.NewConnection(config.Address)
	if err != nil {
		return nil, err
	}

	tlsCert := grpcClient.Certificate()
	tlsCertHash, err := grpc.GetTLSCertHash(&tlsCert)
	if err != nil {
		return nil, err
	}

	vc := &benchmark.ViewClient{
		SignF:       signer.Sign,
		Creator:     signerIdentity,
		TLSCertHash: tlsCertHash,
		Client:      protos.NewViewServiceClient(conn),
	}

	return vc, nil
}
