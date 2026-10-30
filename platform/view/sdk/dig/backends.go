/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"cmp"
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	viewgrpcserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/web"
)

// BackendsGroup is the dig group the SDK collects Backends from.
const BackendsGroup = "view-remote-backends"

// Backend serves the remote view API on one transport. The SDK mounts the backends that
// fsc.view.remote.backends lists, in list order, during Start and before any listener
// starts serving. A developer adds a transport by providing a Backend into BackendsGroup and
// listing its name.
//
// The SDK constructs every backend in the group to read its name, listed or not, so a
// backend's constructor must be free of side effects; Mount acquires what it needs.
type Backend interface {
	// Name is the name fsc.view.remote.backends lists the backend under.
	Name() string
	// Mount makes svc reachable on the backend's transport. It is called at most once.
	Mount(svc viewgrpcserver.Service) error
}

// grpcBackend serves the view service on the fsc.grpc listener.
type grpcBackend struct{ server GRPCServer }

func newGRPCBackend(server GRPCServer) Backend { return &grpcBackend{server: server} }

func (*grpcBackend) Name() string { return "grpc" }

func (b *grpcBackend) Mount(svc viewgrpcserver.Service) error {
	if b.server == nil {
		return errors.New("fsc.grpc listener is not enabled")
	}
	protos.RegisterViewServiceServer(b.server, svc)
	return nil
}

// webBackend serves the view service's HTTP binding on the fsc.web listener. The routes do
// not require a client certificate: the signed command authenticates the caller, as on gRPC.
type webBackend struct {
	server  WebServer
	timeout time.Duration
	// maxRecv is the largest request body or WebSocket message the routes read
	// (fsc.web.maxRecvMsgSize).
	maxRecv int
}

func newWebBackend(server WebServer, c driver.ConfigService) Backend {
	return &webBackend{
		server:  server,
		timeout: RemoteViewTimeout(c),
		maxRecv: cmp.Or(c.GetInt(WebMaxRecvMsgSizeKey), protos.DefaultHTTPMaxRecvMsgSize),
	}
}

func (*webBackend) Name() string { return "web" }

func (b *webBackend) Mount(svc viewgrpcserver.Service) error {
	b.server.RegisterHandler(protos.ProcessCommandHTTPPath,
		otelhttp.NewHandler(web.NewProcessCommandHandler(svc, b.maxRecv, b.timeout), "view-service-process-command"), false)
	b.server.RegisterHandler(protos.StreamCommandHTTPPath,
		otelhttp.NewHandler(web.NewStreamCommandHandler(svc, b.maxRecv), "view-service-stream-command"), false)
	return nil
}

// mountBackends mounts the backends cfg lists on svc, in list order. It mounts nothing when
// the remote view API is disabled. Two backends with the same name, or a listed name no
// backend has, fail before anything is mounted.
func mountBackends(cfg RemoteConfig, backends []Backend, svc viewgrpcserver.Service) error {
	if !cfg.Enabled {
		return nil
	}
	byName := make(map[string]Backend, len(backends))
	for _, b := range backends {
		if _, dup := byName[b.Name()]; dup {
			return errors.Errorf("two backends named [%s] are installed", b.Name())
		}
		byName[b.Name()] = b
	}
	listed := make([]Backend, len(cfg.Backends))
	for i, name := range cfg.Backends {
		b, ok := byName[name]
		if !ok {
			return errors.Errorf("[%s] lists [%s], but no backend of that name is installed", RemoteBackendsKey, name)
		}
		listed[i] = b
	}
	for _, b := range listed {
		if err := b.Mount(svc); err != nil {
			return errors.WithMessagef(err, "failed mounting backend [%s]", b.Name())
		}
	}
	return nil
}

// drainState is the view manager's drain, as the SDK uses it.
type drainState interface {
	Draining() bool
	OnDrain(f func())
}

// ErrDraining is the gRPC Unavailable status a view-API command gets while the node drains;
// the client should retry on another replica.
var ErrDraining = status.Error(codes.Unavailable, "node is draining")

// drainGate wraps the view service so that new commands are refused with ErrDraining while the
// node drains, and reports the drain as a readiness gate. A command that arrived before the
// drain keeps running.
type drainGate struct {
	viewgrpcserver.Service
	state drainState
}

func (g drainGate) ProcessCommand(ctx context.Context, sc *protos.SignedCommand) (*protos.SignedCommandResponse, error) {
	if g.state.Draining() {
		return nil, ErrDraining
	}
	return g.Service.ProcessCommand(ctx, sc)
}

func (g drainGate) StreamCommand(server protos.ViewService_StreamCommandServer) error {
	if g.state.Draining() {
		return ErrDraining
	}
	return g.Service.StreamCommand(server)
}

// NotReady reports "draining" while the node drains.
func (g drainGate) NotReady() (string, bool) { return "draining", g.state.Draining() }
