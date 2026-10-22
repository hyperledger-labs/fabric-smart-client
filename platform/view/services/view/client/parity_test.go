/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	servicesmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/web"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ecdsaID is a signing identity serialized as its PEM certificate, the form
// a node's client allowlist entries take.
type ecdsaID struct {
	key  *ecdsa.PrivateKey
	cert []byte
}

func newID(t *testing.T, cn string) *ecdsaID {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return &ecdsaID{key: key, cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (i *ecdsaID) Serialize() ([]byte, error) { return i.cert, nil }

func (i *ecdsaID) Sign(msg []byte) ([]byte, error) {
	h := sha256.Sum256(msg)
	return ecdsa.SignASN1(rand.Reader, i.key, h[:])
}

func (i *ecdsaID) Verify(msg, sig []byte) error {
	h := sha256.Sum256(msg)
	if !ecdsa.VerifyASN1(&i.key.PublicKey, h[:], sig) {
		return errors.New("invalid signature")
	}
	return nil
}

// keys resolves signers and verifiers for the identities the node knows.
type keys map[string]*ecdsaID

func (k keys) GetVerifier(id view2.Identity) (sig.Verifier, error) {
	if i, ok := k[string(id)]; ok {
		return i, nil
	}
	return nil, errors.New("unknown identity")
}

func (k keys) GetSigner(id view2.Identity) (sig.Signer, error) {
	if i, ok := k[string(id)]; ok {
		return i, nil
	}
	return nil, errors.New("unknown identity")
}

type ids struct {
	node    *ecdsaID
	clients []*ecdsaID
}

func (i ids) DefaultIdentity() view2.Identity { return i.node.cert }
func (i ids) Clients() []view2.Identity {
	out := make([]view2.Identity, len(i.clients))
	for n, c := range i.clients {
		out[n] = c.cert
	}
	return out
}

// Views the suite calls.
type echoView struct{ in []byte }

func (v echoView) Call(view2.Context) (any, error) { return v.in, nil }

type failView struct{}

func (failView) Call(view2.Context) (any, error) { return nil, errors.New("view says no") }

type slowView struct{}

func (slowView) Call(ctx view2.Context) (any, error) {
	<-ctx.Context().Done()
	return nil, ctx.Context().Err()
}

type chatView struct{}

func (chatView) Call(ctx view2.Context) (any, error) {
	s := view.GetStream(ctx)
	var in string
	if err := s.Recv(&in); err != nil {
		return nil, err
	}
	if err := s.Send("echo:" + in); err != nil {
		return nil, err
	}
	return []byte("done"), nil
}

type factory func(in []byte) view2.View

func (f factory) NewView(in []byte) (view2.View, error) { return f(in), nil }

// node is one view service served on both transports.
type node struct {
	grpcAddr, httpAddr string
	allowed            *ecdsaID
	clientTLS          grpc2.SecureOptions
}

func newNode(t *testing.T, serverTLS grpc2.SecureOptions) *node {
	t.Helper()
	nodeID, allowed := newID(t, "node"), newID(t, "client")
	k := keys{string(nodeID.cert): nodeID, string(allowed.cert): allowed}
	idp := ids{node: nodeID, clients: []*ecdsaID{allowed}}

	ip := &mock.IdentityProvider{}
	ip.DefaultIdentityReturns(nodeID.cert)
	registry := view.NewRegistry()
	metrics := view.NewMetrics(&disabled.Provider{})
	cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, &mock.SessionFactory{}, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
	manager := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())
	require.NoError(t, manager.RegisterFactory("echo", factory(func(in []byte) view2.View { return echoView{in: in} })))
	require.NoError(t, manager.RegisterFactory("fail", factory(func([]byte) view2.View { return failView{} })))
	require.NoError(t, manager.RegisterFactory("slow", factory(func([]byte) view2.View { return slowView{} })))
	require.NoError(t, manager.RegisterFactory("chat", factory(func([]byte) view2.View { return chatView{} })))

	marshaller, err := server.NewResponseMarshaler(idp, k)
	require.NoError(t, err)
	svc, err := server.NewViewServiceServer(marshaller, server.NewAccessControlChecker(idp, k),
		server.NewMetrics(&disabled.Provider{}), noop.NewTracerProvider(), server.BindToVerifiedClientCert, 15*time.Minute)
	require.NoError(t, err)
	server.InstallViewHandler(manager, svc, 200*time.Millisecond)

	gs, err := grpc2.NewGRPCServer("127.0.0.1:0", grpc2.ServerConfig{SecOpts: serverTLS})
	require.NoError(t, err)
	protos.RegisterViewServiceServer(gs.Server(), svc)
	go func() { _ = gs.Start() }()
	t.Cleanup(gs.Stop)

	mux := http.NewServeMux()
	mux.Handle(protos.ProcessCommandHTTPPath, web.NewProcessCommandHandler(svc, protos.DefaultHTTPMaxRecvMsgSize, time.Minute))
	mux.Handle(protos.StreamCommandHTTPPath, web.NewStreamCommandHandler(svc, protos.DefaultHTTPMaxRecvMsgSize))
	hs := httptest.NewUnstartedServer(mux)
	if serverTLS.UseTLS {
		cfg, err := serverTLS.TLSConfig()
		require.NoError(t, err)
		hs.TLS = cfg
		hs.StartTLS()
	} else {
		hs.Start()
	}
	t.Cleanup(hs.Close)
	httpAddr := strings.TrimPrefix(strings.TrimPrefix(hs.URL, "https://"), "http://")

	return &node{grpcAddr: gs.Address(), httpAddr: httpAddr, allowed: allowed}
}

func (n *node) client(t *testing.T, transport client.TransportType, id client.SigningIdentity) *client.Client {
	t.Helper()
	addr := n.grpcAddr
	if transport == client.TransportHTTP {
		addr = n.httpAddr
	}
	c, err := client.NewClient(&client.Config{ConnectionConfig: &grpc2.ConnectionConfig{Address: addr, ConnectionTimeout: 5 * time.Second, TLS: n.clientTLS}, Transport: transport}, id, noop.NewTracerProvider())
	require.NoError(t, err)
	return c
}

type outcome struct {
	result string
	err    string
}

// skew matches the clock skew a stale-command error measures, which differs per call.
var skew = regexp.MustCompile(`timestamp is \[[^]]+\] away`)

func outcomeOf(res any, err error) outcome {
	if err != nil {
		return outcome{err: skew.ReplaceAllString(err.Error(), "timestamp is [skew] away")}
	}
	b, _ := res.([]byte)
	return outcome{result: string(b)}
}

var transports = []client.TransportType{client.TransportGRPC, client.TransportHTTP}

// Every scenario gives the same outcome over gRPC and over HTTP.
func TestTransportParity(t *testing.T) {
	t.Parallel()
	n := newNode(t, grpc2.SecureOptions{})
	stranger := newID(t, "stranger")

	scenarios := map[string]func(c *client.Client) outcome{
		"call succeeds":  func(c *client.Client) outcome { return outcomeOf(c.CallView("echo", []byte("hi"))) },
		"view fails":     func(c *client.Client) outcome { return outcomeOf(c.CallView("fail", nil)) },
		"unknown view":   func(c *client.Client) outcome { return outcomeOf(c.CallView("nope", nil)) },
		"view times out": func(c *client.Client) outcome { return outcomeOf(c.CallView("slow", nil)) },
		"unknown identity": func(c *client.Client) outcome {
			c.SigningIdentity = stranger
			return outcomeOf(c.CallView("echo", nil))
		},
		"stale command": func(c *client.Client) outcome {
			c.Time = func() time.Time { return time.Now().Add(-time.Hour) }
			return outcomeOf(c.CallView("echo", nil))
		},
		"stream": func(c *client.Client) outcome {
			s, err := c.StreamCallView("chat", nil)
			if err != nil {
				return outcome{err: err.Error()}
			}
			if err := s.Send("ping"); err != nil {
				return outcome{err: err.Error()}
			}
			var got string
			if err := s.Recv(&got); err != nil {
				return outcome{err: err.Error()}
			}
			// Sent after the view's last Recv: the final response must still arrive.
			_ = s.Send("late")
			res, err := s.Result()
			return outcomeOf(append([]byte(got+"|"), res...), err)
		},
		"failing stream": func(c *client.Client) outcome {
			s, err := c.StreamCallView("fail", nil)
			if err != nil {
				return outcome{err: err.Error()}
			}
			return outcomeOf(s.Result())
		},
	}
	expect := map[string]string{
		"call succeeds":    "result:hi",
		"view fails":       "view says no",
		"unknown view":     "no factory found for id [nope]",
		"view times out":   "view execution timed out",
		"unknown identity": "not recognized",
		"stale command":    "away from local time",
		"stream":           "result:echo:ping|done",
		"failing stream":   "view says no",
	}
	for name, run := range scenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := map[client.TransportType]outcome{}
			for _, tr := range transports {
				got[tr] = run(n.client(t, tr, n.allowed))
			}
			require.Equal(t, got[client.TransportGRPC], got[client.TransportHTTP])
			// The shared outcome is the expected one, not a failure both transports share.
			if want := expect[name]; strings.HasPrefix(want, "result:") {
				require.Equal(t, strings.TrimPrefix(want, "result:"), got[client.TransportGRPC].result)
			} else {
				require.Contains(t, got[client.TransportGRPC].err, want)
			}
		})
	}
}

// A command accepted on one transport is a replay on the other.
func TestReplayAcrossTransports(t *testing.T) {
	t.Parallel()
	n := newNode(t, grpc2.SecureOptions{})
	c := n.client(t, client.TransportHTTP, n.allowed)
	sc, err := c.CreateSignedCommand(&protos.Command_CallView{CallView: &protos.CallView{Fid: "echo"}}, n.allowed)
	require.NoError(t, err)

	for i, tr := range transports {
		svc, closer, err := n.client(t, tr, n.allowed).Transport.Open()
		require.NoError(t, err)
		scr, err := svc.ProcessCommand(t.Context(), sc)
		require.NoError(t, err)
		_ = closer.Close()
		cr := &protos.CommandResponse{}
		require.NoError(t, proto.Unmarshal(scr.Response, cr))
		if i == 0 {
			require.Nil(t, cr.GetErr(), "first delivery is accepted")
		} else {
			require.Contains(t, cr.GetErr().GetMessage(), "command already processed")
		}
	}
}

// wrongCert presents one certificate on the connection but names another in commands.
type wrongCert struct {
	client.Transport
	other *tls.Certificate
}

func (w wrongCert) Certificate() *tls.Certificate { return w.other }

func TestBindingParity(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	srvKP, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	cliKP, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	otherKP, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	other, err := tls.X509KeyPair(otherKP.Cert, otherKP.Key)
	require.NoError(t, err)

	n := newNode(t, grpc2.SecureOptions{
		UseTLS: true, Certificate: srvKP.Cert, Key: srvKP.Key,
		RequireClientCert: true, ClientRootCAs: [][]byte{ca.CertBytes()},
	})
	n.clientTLS = grpc2.SecureOptions{
		UseTLS: true, ServerRootCAs: [][]byte{ca.CertBytes()},
		RequireClientCert: true, Certificate: cliKP.Cert, Key: cliKP.Key,
	}

	ok, bad := map[client.TransportType]outcome{}, map[client.TransportType]outcome{}
	for _, tr := range transports {
		c := n.client(t, tr, n.allowed)
		ok[tr] = outcomeOf(c.CallView("echo", []byte("hi")))
		c.Transport = wrongCert{Transport: c.Transport, other: &other}
		bad[tr] = outcomeOf(c.CallView("echo", nil))
	}
	require.Equal(t, outcome{result: "hi"}, ok[client.TransportGRPC])
	require.Equal(t, ok[client.TransportGRPC], ok[client.TransportHTTP])
	require.Contains(t, bad[client.TransportGRPC].err, "does not name the client TLS certificate")
	require.Equal(t, bad[client.TransportGRPC], bad[client.TransportHTTP])
}
