/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/testpb"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

// inFlightHandler stands in for a view that ignores cancellation: once entered, it blocks
// until released and then writes to the KVS.
type inFlightHandler struct {
	kvs              *kvs.KVS
	entered, release chan struct{}
	err              chan error
}

func newInFlightHandler(t *testing.T) *inFlightHandler {
	t.Helper()
	p := providerFrom(t, "")
	kvsInst, err := newKVS(p, newMuxDriver(p, mem.NewNamedDriver(sqlite2.NewDbProvider())))
	require.NoError(t, err)
	return &inFlightHandler{kvs: kvsInst, entered: make(chan struct{}), release: make(chan struct{}), err: make(chan error, 1)}
}

func (h *inFlightHandler) handle(ctx context.Context) {
	close(h.entered)
	<-h.release
	h.err <- h.kvs.Put(context.WithoutCancel(ctx), "k", "v")
}

// requireKVSClosedAfterHandler checks that the handler's write succeeded and that shutdown
// then went on to close the KVS.
func (h *inFlightHandler) requireKVSClosedAfterHandler(t *testing.T) {
	t.Helper()
	require.NoError(t, <-h.err, "the KVS must stay open until in-flight handlers return")
	require.Eventually(t, func() bool {
		return h.kvs.Put(context.Background(), "probe", "v") != nil
	}, 5*time.Second, 50*time.Millisecond, "KVS was never stopped")
}

// TestServe_KVSOutlivesInFlightWebRequest uses the real web server, whose Start returns once
// Serve is spawned, and a handler that outlives the web server's shutdown grace period.
func TestServe_KVSOutlivesInFlightWebRequest(t *testing.T) { //nolint:paralleltest // relies on shutdown timing
	h := newInFlightHandler(t)
	ws := web.NewServer(web.Options{ListenAddress: "127.0.0.1:0"})
	ws.RegisterHandler("/slow", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		h.handle(r.Context())
	}), false)

	ctx, cancel := context.WithCancel(context.Background())
	startServe(nil, ws, OperationsServer{}, nil, h.kvs, ctx).Wait() // web listener is bound
	go func() {
		resp, err := http.Get("http://" + ws.Addr() + "/slow")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-h.entered

	cancel()
	time.AfterFunc(6*time.Second, func() { close(h.release) }) // past the 5s grace period
	h.requireKVSClosedAfterHandler(t)
}

type inFlightEmptyService struct {
	testpb.UnimplementedEmptyServiceServer
	*inFlightHandler
}

func (s inFlightEmptyService) EmptyCall(ctx context.Context, _ *testpb.Empty) (*testpb.Empty, error) {
	s.handle(ctx)
	return &testpb.Empty{}, nil
}

// TestServe_KVSOutlivesInFlightGRPCCall checks the same for a unary RPC, whose handler keeps
// running after grpc.Server.Stop cancelled its context and closed its connection.
func TestServe_KVSOutlivesInFlightGRPCCall(t *testing.T) { //nolint:paralleltest // relies on shutdown timing
	h := newInFlightHandler(t)
	gs, err := grpc2.NewGRPCServer("127.0.0.1:0", grpc2.ServerConfig{})
	require.NoError(t, err)
	testpb.RegisterEmptyServiceServer(gs.Server(), inFlightEmptyService{inFlightHandler: h})

	ctx, cancel := context.WithCancel(context.Background())
	startServe(gs, newFakeServer(), OperationsServer{}, nil, h.kvs, ctx)
	cc, err := grpc.NewClient(gs.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = cc.Close() }()
	callErr := make(chan error, 1)
	go func() {
		_, err := testpb.NewEmptyServiceClient(cc).EmptyCall(context.Background(), &testpb.Empty{})
		callErr <- err
	}()
	<-h.entered

	cancel()
	require.Error(t, <-callErr, "Stop closes the connection of the in-flight call")
	time.Sleep(200 * time.Millisecond) // let shutdown get as far as it would without the wait
	close(h.release)
	h.requireKVSClosedAfterHandler(t)
}
