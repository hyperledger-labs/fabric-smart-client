/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

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
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestConfigJSON(t *testing.T) {
	t.Parallel()
	cfg := Config{
		ID: "test-id",
		ConnectionConfig: &grpc2.ConnectionConfig{
			Address: "localhost:1234",
		},
	}

	// JSON marshaling
	b, err := cfg.ToJSon()
	require.NoError(t, err)
	require.NotNil(t, b)

	cfgs := Configs{cfg}
	b2, err := cfgs.ToJSon()
	require.NoError(t, err)
	require.NotNil(t, b2)

	loaded, err := FromJSON(b2)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Equal(t, "test-id", loaded[0].ID)

	// Bad JSON unmarshaling
	_, err = FromJSON([]byte("not-json"))
	require.Error(t, err)
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	tp := noop.NewTracerProvider()
	cfg := &Config{
		ConnectionConfig: &grpc2.ConnectionConfig{
			Address: "localhost:1234",
		},
	}
	sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
	cl, err := NewClient(cfg, sid, tp)
	require.NoError(t, err)
	require.NotNil(t, cl)
}

func TestClient_CallView(t *testing.T) {
	t.Parallel()
	tp := noop.NewTracerProvider()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("view-result")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		res, err := cl.CallView("fid", []byte("input"))
		require.NoError(t, err)
		require.Equal(t, []byte("view-result"), res)
	})

	t.Run("Signing error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("view-result")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig"), signErr: fmt.Errorf("sign error")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		_, err := cl.CallView("fid", []byte("input"))
		require.ErrorContains(t, err, "sign error")
	})

	t.Run("processCommand error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("view-result"), processErr: fmt.Errorf("process error")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		_, err := cl.CallView("fid", []byte("input"))
		require.ErrorContains(t, err, "process error")
	})

	t.Run("missing InitiateView response", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("view-result"), respBytes: []byte("junk-proto")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		_, err := cl.CallView("fid", []byte("input"))
		require.Error(t, err)
	})

	t.Run("context cancellation awareness", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("view-result")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := cl.CallViewWithContext(ctx, "fid", []byte("input"))
		require.Error(t, err)
	})
}

func TestClient_StreamCallView(t *testing.T) {
	t.Parallel()
	tp := noop.NewTracerProvider()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("stream-result")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		st, err := cl.StreamCallView("fid", []byte("input"))
		require.NoError(t, err)
		require.NotNil(t, st)
	})

	t.Run("Signing error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("stream-result")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig"), signErr: fmt.Errorf("sign error")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		_, err := cl.StreamCallView("fid", []byte("input"))
		require.ErrorContains(t, err, "sign error")
	})

	t.Run("streamCommand error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("stream-result"), streamErr: fmt.Errorf("stream error")}
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			Address: "localhost",
			ViewServiceClient: &fakeStreamClient{
				vsc: vsc,
			},
			RandomnessReader: rand.Reader,
			Time:             time.Now,
			SigningIdentity:  sid,
			tracer:           tp.Tracer("test"),
		}
		_, err := cl.StreamCallView("fid", []byte("input"))
		require.ErrorContains(t, err, "stream error")
	})
}

func TestClient_ProcessCommand(t *testing.T) {
	t.Parallel()
	sc := &protos.SignedCommand{}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res")}
		fc := &fakeStreamClient{vsc: vsc}
		cl := &client{ViewServiceClient: fc}
		resp, err := cl.processCommand(t.Context(), sc)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("CreateViewClient error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res")}
		fc := &fakeStreamClient{vsc: vsc, createErr: fmt.Errorf("create view client error")}
		cl := &client{ViewServiceClient: fc}
		_, err := cl.processCommand(t.Context(), sc)
		require.ErrorContains(t, err, "create view client error")
	})

	t.Run("unmarshal error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res"), respBytes: []byte("invalid-bytes")}
		fc := &fakeStreamClient{vsc: vsc}
		cl := &client{ViewServiceClient: fc}
		_, err := cl.processCommand(t.Context(), sc)
		require.ErrorContains(t, err, "failed to unmarshal")
	})

	t.Run("View error payload returned", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res"), cmdRespErr: &protos.Error{Message: "view execution exception"}}
		fc := &fakeStreamClient{vsc: vsc}
		cl := &client{ViewServiceClient: fc}
		_, err := cl.processCommand(t.Context(), sc)
		require.ErrorContains(t, err, "view execution exception")
	})
}

func TestClient_StreamCommand(t *testing.T) {
	t.Parallel()
	sc := &protos.SignedCommand{}

	t.Run("CreateViewClient error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res")}
		fc := &fakeStreamClient{vsc: vsc, createErr: fmt.Errorf("create view client error")}
		cl := &client{ViewServiceClient: fc}
		_, _, err := cl.streamCommand(t.Context(), sc)
		require.Error(t, err)
	})

	t.Run("Send error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte("res"), sendErr: fmt.Errorf("send error")}
		fc := &fakeStreamClient{vsc: vsc}
		cl := &client{ViewServiceClient: fc}
		_, _, err := cl.streamCommand(t.Context(), sc)
		require.ErrorContains(t, err, "failed to send signed command")
	})
}

func TestClient_CreateSignedCommand(t *testing.T) {
	t.Parallel()

	t.Run("Invalid payload", func(t *testing.T) {
		t.Parallel()
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			ViewServiceClient: &fakeStreamClient{},
			RandomnessReader:  rand.Reader,
			Time:              time.Now,
			SigningIdentity:   sid,
		}
		_, err := cl.CreateSignedCommand("not-a-command", sid)
		require.ErrorContains(t, err, "command type not recognized")
	})

	t.Run("Randomness reader error", func(t *testing.T) {
		t.Parallel()
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			ViewServiceClient: &fakeStreamClient{},
			RandomnessReader:  &failingReader{},
			Time:              time.Now,
			SigningIdentity:   sid,
		}
		payload := &protos.Command_CallView{CallView: &protos.CallView{Fid: "fid"}}
		_, err := cl.CreateSignedCommand(payload, sid)
		require.Error(t, err)
	})

	t.Run("Serialize error", func(t *testing.T) {
		t.Parallel()
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig"), serializeErr: fmt.Errorf("serialize error")}
		cl := &client{
			ViewServiceClient: &fakeStreamClient{},
			RandomnessReader:  rand.Reader,
			Time:              time.Now,
			SigningIdentity:   sid,
		}
		payload := &protos.Command_CallView{CallView: &protos.CallView{Fid: "fid"}}
		_, err := cl.CreateSignedCommand(payload, sid)
		require.Error(t, err)
	})

	t.Run("Success path covering InitiateView payload mapping", func(t *testing.T) {
		t.Parallel()
		sid := &fakeSigningIdentity{serialized: []byte("creator"), signature: []byte("sig")}
		cl := &client{
			ViewServiceClient: &fakeStreamClient{},
			RandomnessReader:  rand.Reader,
			Time:              time.Now,
			SigningIdentity:   sid,
		}
		initPayload := &protos.Command_InitiateView{InitiateView: &protos.InitiateView{Fid: "fid"}}
		sc, err := cl.CreateSignedCommand(initPayload, sid)
		require.NoError(t, err)
		require.NotNil(t, sc)
	})
}

func TestStream(t *testing.T) {
	t.Parallel()

	t.Run("Send", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`)}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		err = st.Send("test-msg")
		require.NoError(t, err)
	})

	t.Run("Send error marshaling", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`)}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		err = st.Send(make(chan int)) // unmarshalable
		require.Error(t, err)
	})

	t.Run("Recv", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`)}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		var s string
		err = st.Recv(&s)
		require.NoError(t, err)
		require.Equal(t, "test-value", s)
	})

	t.Run("SendProtoMsg and RecvProtoMsg", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`)}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		err = st.SendProtoMsg(&protos.CallViewResponse{})
		require.NoError(t, err)
		err = st.RecvProtoMsg(&protos.CallViewResponse{})
		require.NoError(t, err)
	})

	t.Run("Result success", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`)}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		res, err := st.Result()
		require.NoError(t, err)
		require.Equal(t, []byte(`"test-value"`), res)
	})

	t.Run("Result recv error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`), recvErr: fmt.Errorf("recv error")}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		_, err = st.Result()
		require.ErrorContains(t, err, "recv error")
	})

	t.Run("Result unmarshal error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`), respBytes: []byte("junk")}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		_, err = st.Result()
		require.Error(t, err)
	})

	t.Run("Result view error", func(t *testing.T) {
		t.Parallel()
		vsc := &fakeProtosVSC{cvrBytes: []byte(`"test-value"`), cmdRespErr: &protos.Error{Message: "view error"}}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		_, err = st.Result()
		require.ErrorContains(t, err, "view error")
	})

	t.Run("Result empty cvr", func(t *testing.T) {
		t.Parallel()
		emptyResp := &protos.CommandResponse{
			Payload: &protos.CommandResponse_InitiateViewResponse{
				InitiateViewResponse: &protos.InitiateViewResponse{},
			},
		}
		respBytes, _ := proto.Marshal(emptyResp)
		vsc := &fakeProtosVSC{respBytes: respBytes}
		dummyConn, err := grpc.NewClient("passthrough:///dummy", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		t.Cleanup(func() { _ = dummyConn.Close() })
		st := &Stream{
			scc:  &fakeBidiStream{f: vsc},
			conn: dummyConn,
		}
		_, err = st.Result()
		require.ErrorContains(t, err, "no call view response found")
	})
}

func TestNewX509SigningIdentity(t *testing.T) {
	t.Parallel()
	certPath, skPath, err := createTempIdentityFiles()
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(filepath.Dir(certPath)) }()

	sid, err := NewX509SigningIdentity(certPath, skPath)
	require.NoError(t, err)
	require.NotNil(t, sid)

	// Test Signing and Serialize methods on created signingIdentity
	serialized, err := sid.Serialize()
	require.NoError(t, err)
	require.NotNil(t, serialized)

	sigBytes, err := sid.Sign([]byte("msg"))
	require.NoError(t, err)
	require.NotNil(t, sigBytes)

	// Missing cert file
	_, err = NewX509SigningIdentity("non-existent-cert", skPath)
	require.Error(t, err)

	// Missing sk file
	_, err = NewX509SigningIdentity(certPath, "non-existent-sk")
	require.Error(t, err)

	// Empty sk file (decode failure)
	badSkPath := filepath.Join(filepath.Dir(skPath), "bad_sk.pem")
	_ = os.WriteFile(badSkPath, []byte("not-a-pem"), 0o644)
	_, err = NewX509SigningIdentity(certPath, badSkPath)
	require.ErrorContains(t, err, "failed decoding PEM")

	// Invalid PKCS8 key block bytes
	badPemBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad-key-bytes")})
	_ = os.WriteFile(badSkPath, badPemBlock, 0o644)
	_, err = NewX509SigningIdentity(certPath, badSkPath)
	require.ErrorContains(t, err, "failed parsing PKCS8")
}

func TestLocalClient(t *testing.T) {
	t.Parallel()
	sp := &fakeServiceProvider{
		registry: view.NewRegistry(),
	}
	// Register a fake view factory to trigger NewView success
	_ = sp.registry.RegisterFactory("fid", &fakeFactory{})

	vm := view.NewManager(
		&fakeIdentityProvider{},
		sp.registry,
		&view.Metrics{Contexts: &fakeGauge{}},
		&fakeContextFactory{ctx: &fakeParentContext{id: "ctx1"}},
		&fakeRunner{res: []byte("local-res")},
	)
	sp.vm = vm

	lc := NewLocalClient(sp)
	res, err := lc.CallView("fid", []byte("input"))
	require.NoError(t, err)
	require.Equal(t, []byte("local-res"), res)

	// Test marshaling of non-byte runner result
	vmStruct := view.NewManager(
		&fakeIdentityProvider{},
		sp.registry,
		&view.Metrics{Contexts: &fakeGauge{}},
		&fakeContextFactory{ctx: &fakeParentContext{id: "ctx2"}},
		&fakeRunner{res: map[string]string{"status": "ok"}},
	)
	sp.vm = vmStruct
	lcStruct := NewLocalClient(sp)
	resStruct, err := lcStruct.CallView("fid", []byte("input"))
	require.NoError(t, err)
	expectedJSON, _ := json.Marshal(map[string]string{"status": "ok"})
	require.Equal(t, expectedJSON, resStruct)

	// getTracer error
	spErr := &fakeServiceProvider{tpErr: fmt.Errorf("tracer provider error")}
	lcErr := NewLocalClient(spErr)
	_, err = lcErr.CallView("fid", []byte("input"))
	require.ErrorContains(t, err, "tracer provider error")

	// GetManager error
	spErrVM := &fakeServiceProvider{vmErr: fmt.Errorf("view manager error")}
	lcErrVM := NewLocalClient(spErrVM)
	// initialize lcErrVM tracer to avoid getTracer failure
	_, _ = lcErrVM.CallView("fid", []byte("input")) // fails at manager lookup
	_, err = lcErrVM.CallView("fid", []byte("input"))
	require.ErrorContains(t, err, "view manager error")

	// NewView error
	sp.registry = view.NewRegistry() // unregisters factory
	sp.vm = view.NewManager(
		&fakeIdentityProvider{},
		sp.registry,
		&view.Metrics{Contexts: &fakeGauge{}},
		&fakeContextFactory{ctx: &fakeParentContext{id: "ctx3"}},
		&fakeRunner{res: []byte("local-res")},
	)
	_, err = lc.CallView("missing-fid", []byte("input"))
	require.ErrorContains(t, err, "failed instantiating view")

	// InitiateView error
	_ = sp.registry.RegisterFactory("failing-run", &fakeFactory{})
	sp.vm = view.NewManager(
		&fakeIdentityProvider{},
		sp.registry,
		&view.Metrics{Contexts: &fakeGauge{}},
		&fakeContextFactory{ctx: &fakeParentContext{id: "ctx4"}},
		&fakeRunner{err: fmt.Errorf("runner execution fault")},
	)
	_, err = lc.CallView("failing-run", []byte("input"))
	require.ErrorContains(t, err, "runner execution fault")
}

type fakeStreamClient struct {
	vsc       *fakeProtosVSC
	createErr error
}

func (f *fakeStreamClient) CreateViewClient() (*grpc.ClientConn, protos.ViewServiceClient, error) {
	if f.createErr != nil {
		return nil, nil, f.createErr
	}
	return nil, f.vsc, nil
}

func (f *fakeStreamClient) Certificate() *tls.Certificate {
	return nil
}

type fakeProtosVSC struct {
	processErr error
	streamErr  error
	sendErr    error
	recvErr    error
	sendMsgErr error
	recvMsgErr error
	respBytes  []byte
	cvrBytes   []byte
	cmdRespErr *protos.Error
}

func (f *fakeProtosVSC) ProcessCommand(ctx context.Context, in *protos.SignedCommand, opts ...grpc.CallOption) (*protos.SignedCommandResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if f.processErr != nil {
		return nil, f.processErr
	}
	resp := &protos.CommandResponse{
		Payload: &protos.CommandResponse_CallViewResponse{
			CallViewResponse: &protos.CallViewResponse{Result: f.cvrBytes},
		},
	}
	if f.cmdRespErr != nil {
		resp.Payload = &protos.CommandResponse_Err{Err: f.cmdRespErr}
	}
	b := f.respBytes
	if b == nil {
		b, _ = proto.Marshal(resp)
	}
	return &protos.SignedCommandResponse{Response: b}, nil
}

func (f *fakeProtosVSC) StreamCommand(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[protos.SignedCommand, protos.SignedCommandResponse], error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return &fakeBidiStream{f: f}, nil
}

type fakeBidiStream struct {
	f *fakeProtosVSC
	grpc.ClientStream
}

func (s *fakeBidiStream) Send(m *protos.SignedCommand) error {
	return s.f.sendErr
}

func (s *fakeBidiStream) Recv() (*protos.SignedCommandResponse, error) {
	if s.f.recvErr != nil {
		return nil, s.f.recvErr
	}
	resp := &protos.CommandResponse{
		Payload: &protos.CommandResponse_CallViewResponse{
			CallViewResponse: &protos.CallViewResponse{Result: s.f.cvrBytes},
		},
	}
	if s.f.cmdRespErr != nil {
		resp.Payload = &protos.CommandResponse_Err{Err: s.f.cmdRespErr}
	}
	b := s.f.respBytes
	if b == nil {
		b, _ = proto.Marshal(resp)
	}
	return &protos.SignedCommandResponse{Response: b}, nil
}

func (s *fakeBidiStream) CloseSend() error { return nil }

func (s *fakeBidiStream) SendMsg(m any) error {
	return s.f.sendMsgErr
}

func (s *fakeBidiStream) RecvMsg(m any) error {
	if s.f.recvMsgErr != nil {
		return s.f.recvMsgErr
	}
	if cvr, ok := m.(*protos.CallViewResponse); ok {
		cvr.Result = s.f.cvrBytes
	}
	return nil
}

type fakeSigningIdentity struct {
	serializeErr error
	signErr      error
	serialized   []byte
	signature    []byte
}

func (f *fakeSigningIdentity) Serialize() ([]byte, error) {
	return f.serialized, f.serializeErr
}

func (f *fakeSigningIdentity) Sign(msg []byte) ([]byte, error) {
	return f.signature, f.signErr
}

type failingReader struct{}

func (r *failingReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func createTempIdentityFiles() (string, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	skPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDer, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})

	dir := filepath.Join(os.TempDir(), fmt.Sprintf("id-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(dir, 0o755)
	certPath := filepath.Join(dir, "cert.pem")
	skPath := filepath.Join(dir, "sk.pem")
	_ = os.WriteFile(certPath, certPEM, 0o644)
	_ = os.WriteFile(skPath, skPEM, 0o644)
	return certPath, skPath, nil
}

type fakeServiceProvider struct {
	tpErr    error
	vmErr    error
	vm       *view.Manager
	registry *view.Registry
}

func (f *fakeServiceProvider) GetService(v any) (any, error) {
	t, ok := v.(reflect.Type)
	if !ok {
		return nil, fmt.Errorf("expected reflect.Type")
	}
	if t == reflect.TypeOf((*tracing.Provider)(nil)) {
		if f.tpErr != nil {
			return nil, f.tpErr
		}
		return noop.NewTracerProvider(), nil
	}
	if t == reflect.TypeOf((*view.Manager)(nil)) {
		if f.vmErr != nil {
			return nil, f.vmErr
		}
		return f.vm, nil
	}
	if t == reflect.TypeOf((*view.Registry)(nil)) {
		return f.registry, nil
	}
	return nil, fmt.Errorf("service not found: %v", t)
}

type fakeFactory struct{}

func (f *fakeFactory) NewView(in []byte) (view2.View, error) {
	return &fakeView{}, nil
}

type fakeView struct{}

func (f *fakeView) Call(context view2.Context) (interface{}, error) {
	return nil, nil
}

type fakeIdentityProvider struct{}

func (f *fakeIdentityProvider) Identity(s string) view2.Identity { return nil }
func (f *fakeIdentityProvider) DefaultIdentity() view2.Identity  { return nil }

type fakeGauge struct{}

func (f *fakeGauge) With(labelValues ...string) metrics.Gauge { return f }
func (f *fakeGauge) Add(delta float64)                        {}
func (f *fakeGauge) Set(value float64)                        {}

type fakeContextFactory struct {
	err error
	ctx view.ParentContext
}

func (f *fakeContextFactory) NewForInitiator(ctx context.Context, contextID string, id view2.Identity, v view2.View) (view.ParentContext, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ctx, nil
}

func (f *fakeContextFactory) NewForResponder(ctx context.Context, contextID string, me view2.Identity, session view2.Session, party view2.Identity) (view.ParentContext, error) {
	return nil, nil
}

type fakeParentContext struct {
	id string
}

func (f *fakeParentContext) ID() string { return f.id }
func (f *fakeParentContext) StartSpanFrom(c context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return c, trace.SpanFromContext(c)
}
func (f *fakeParentContext) GetService(v any) (any, error) { return nil, nil }
func (f *fakeParentContext) RunView(v view2.View, opts ...view2.RunViewOption) (any, error) {
	return nil, nil
}
func (f *fakeParentContext) Me() view2.Identity          { return nil }
func (f *fakeParentContext) IsMe(id view2.Identity) bool { return false }
func (f *fakeParentContext) Initiator() view2.View       { return nil }
func (f *fakeParentContext) GetSession(c view2.View, p view2.Identity, b ...view2.View) (view2.Session, error) {
	return nil, nil
}

func (f *fakeParentContext) GetSessionByID(id string, p view2.Identity) (view2.Session, error) {
	return nil, nil
}
func (f *fakeParentContext) Session() view2.Session   { return nil }
func (f *fakeParentContext) Context() context.Context { return context.Background() }
func (f *fakeParentContext) OnError(cb func())        {}
func (f *fakeParentContext) Dispose()                 {}
func (f *fakeParentContext) PutSessionByID(viewID string, party view2.Identity, session view2.Session) error {
	return nil
}
func (f *fakeParentContext) Cleanup() {}
func (f *fakeParentContext) PutSession(caller view2.View, party view2.Identity, session view2.Session) error {
	return nil
}

type fakeRunner struct {
	res any
	err error
}

func (f *fakeRunner) RunView(viewCtx view2.Context, responder view2.View) (any, error) {
	return f.res, f.err
}
