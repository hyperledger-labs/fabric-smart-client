/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/dig"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	viewgrpcserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// fakeGRPCServer records the services registered on it and how often it was stopped.
type fakeGRPCServer struct {
	services []string
	stopped  int
}

func (f *fakeGRPCServer) RegisterService(desc *grpc.ServiceDesc, _ any) {
	f.services = append(f.services, desc.ServiceName)
}
func (*fakeGRPCServer) Start() error { return nil }
func (f *fakeGRPCServer) Stop()      { f.stopped++ }

// recordingWebServer records the patterns registered on it and whether they were secure.
type recordingWebServer struct{ secure map[string]bool }

func (r *recordingWebServer) RegisterHandler(p string, _ http.Handler, secure bool) {
	if r.secure == nil {
		r.secure = map[string]bool{}
	}
	r.secure[p] = secure
}
func (*recordingWebServer) Start() error { return nil }
func (*recordingWebServer) Stop() error  { return nil }

// fakeBackend counts its mounts and fails Mount with err.
type fakeBackend struct {
	name    string
	mounted int
	err     error
}

func (b *fakeBackend) Name() string { return b.name }
func (b *fakeBackend) Mount(viewgrpcserver.Service) error {
	b.mounted++
	return b.err
}

func TestDisabledListenerNotConstructed(t *testing.T) {
	t.Parallel()
	calls := 0
	ctor := GRPCServerConstructor(func(driver.ConfigService) (GRPCServer, error) {
		calls++
		return &fakeGRPCServer{}, nil
	})
	s, err := enabledGRPCServer(ctor, providerFrom(t, "fsc:\n  grpc:\n    enabled: false\n"))
	require.NoError(t, err)
	require.Nil(t, s)
	require.Zero(t, calls)

	webCalls := 0
	webCtor := WebServerConstructor(func(driver.ConfigService) (WebServer, error) {
		webCalls++
		return &recordingWebServer{}, nil
	})
	ws, err := enabledWebServer(webCtor, providerFrom(t, "fsc:\n  web:\n    enabled: false\n"))
	require.NoError(t, err)
	require.NotNil(t, ws, "a disabled web listener is a no-op server, so operations can still register")
	require.Zero(t, webCalls)
}

func TestEnabledListenerNilFails(t *testing.T) {
	t.Parallel()
	nilCtor := GRPCServerConstructor(func(driver.ConfigService) (GRPCServer, error) { return nil, nil })
	_, err := enabledGRPCServer(nilCtor, providerFrom(t, "fsc:\n  grpc:\n    enabled: true\n"))
	require.ErrorContains(t, err, "fsc.grpc.enabled")

	nilWeb := WebServerConstructor(func(driver.ConfigService) (WebServer, error) { return nil, nil })
	_, err = enabledWebServer(nilWeb, providerFrom(t, "fsc:\n  web:\n    enabled: true\n"))
	require.ErrorContains(t, err, "fsc.web.enabled")
}

// A developer replaces a listener by decorating its constructor; the built-in backend then
// mounts the view service on the replacement.
func TestDecoratedListenerReplacesOurs(t *testing.T) {
	t.Parallel()
	replacement := &fakeGRPCServer{}
	c := dig.New()
	require.NoError(t, errors.Join(
		c.Provide(func() driver.ConfigService {
			return providerFrom(t, "fsc:\n  grpc:\n    enabled: true\n")
		}),
		c.Provide(func() GRPCServerConstructor { return NewGRPCServer }),
		c.Provide(enabledGRPCServer),
		c.Provide(newGRPCBackend, dig.Group(BackendsGroup)),
		c.Decorate(func(GRPCServerConstructor) GRPCServerConstructor {
			return func(driver.ConfigService) (GRPCServer, error) { return replacement, nil }
		}),
	))
	require.NoError(t, c.Invoke(func(in struct {
		dig.In
		Backends []Backend `group:"view-remote-backends"`
	},
	) error {
		return mountBackends(RemoteConfig{Enabled: true, Backends: []string{"grpc"}}, in.Backends, &fakeViewService{})
	}))
	require.Equal(t, []string{protos.ViewService_ServiceDesc.ServiceName}, replacement.services)
}

func TestWebBackendMountsViewRoutesWithoutRequireCert(t *testing.T) {
	t.Parallel()
	ws := &recordingWebServer{}
	b := newWebBackend(ws, providerFrom(t, ""))
	require.NoError(t, b.Mount(&fakeViewService{}))
	require.Equal(t, map[string]bool{
		protos.ProcessCommandHTTPPath: false,
		protos.StreamCommandHTTPPath:  false,
	}, ws.secure)
}

// The web backend stops reading a body at fsc.web.maxRecvMsgSize, its listener's own limit,
// and at the HTTP binding's default when that is unset.
func TestWebBackendMaxRecvMsgSize(t *testing.T) {
	t.Parallel()
	require.Equal(t, protos.DefaultHTTPMaxRecvMsgSize, newWebBackend(&recordingWebServer{}, providerFrom(t, "")).(*webBackend).maxRecv)

	ws := &muxWebServer{mux: http.NewServeMux()}
	b := newWebBackend(ws, providerFrom(t, "fsc:\n  web:\n    maxRecvMsgSize: 1024\n"))
	require.NoError(t, b.Mount(&fakeViewService{}))
	ts := httptest.NewServer(ws.mux)
	t.Cleanup(ts.Close)
	resp, err := http.Post(ts.URL+protos.ProcessCommandHTTPPath, "application/json", bytes.NewReader(bytes.Repeat([]byte("a"), 1025)))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// muxWebServer mounts handlers on mux.
type muxWebServer struct{ mux *http.ServeMux }

func (m *muxWebServer) RegisterHandler(p string, h http.Handler, _ bool) { m.mux.Handle(p, h) }
func (*muxWebServer) Start() error                                       { return nil }
func (*muxWebServer) Stop() error                                        { return nil }

func TestMountBackends(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		cfg      RemoteConfig
		backends []*fakeBackend
		mounted  []int
		wantErr  []string
	}{
		{
			name:     "remote disabled mounts nothing",
			cfg:      RemoteConfig{Backends: []string{"a"}},
			backends: []*fakeBackend{{name: "a"}},
			mounted:  []int{0},
		},
		{
			name:     "only listed backends are mounted",
			cfg:      RemoteConfig{Enabled: true, Backends: []string{"a"}},
			backends: []*fakeBackend{{name: "a"}, {name: "extra"}},
			mounted:  []int{1, 0},
		},
		{
			name:     "unknown backend",
			cfg:      RemoteConfig{Enabled: true, Backends: []string{"a", "nats"}},
			backends: []*fakeBackend{{name: "a"}},
			wantErr:  []string{"nats", "fsc.view.remote.backends"},
		},
		{
			name:     "duplicate backend name",
			cfg:      RemoteConfig{Enabled: true, Backends: []string{"a"}},
			backends: []*fakeBackend{{name: "a"}, {name: "a"}},
			wantErr:  []string{"two backends named [a]"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			backends := make([]Backend, len(tc.backends))
			for i, b := range tc.backends {
				backends[i] = b
			}
			err := mountBackends(tc.cfg, backends, &fakeViewService{})
			for _, want := range tc.wantErr {
				require.ErrorContains(t, err, want)
			}
			if len(tc.wantErr) > 0 {
				for _, b := range tc.backends {
					require.Zero(t, b.mounted, "no backend is mounted when the set is invalid")
				}
				return
			}
			require.NoError(t, err)
			for i, b := range tc.backends {
				require.Equal(t, tc.mounted[i], b.mounted, b.name)
			}
		})
	}
}

func TestMountBackendsMountError(t *testing.T) {
	t.Parallel()
	b := &fakeBackend{name: "a", err: errors.New("no route")}
	err := mountBackends(RemoteConfig{Enabled: true, Backends: []string{"a"}}, []Backend{b}, &fakeViewService{})
	require.ErrorContains(t, err, "failed mounting backend [a]")
	require.ErrorContains(t, err, "no route")
}
