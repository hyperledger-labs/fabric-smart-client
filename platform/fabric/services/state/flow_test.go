/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockSession struct {
	recv    <-chan *view.Message
	sendErr error
	sent    [][]byte
}

func (m *mockSession) Info() view.SessionInfo { return view.SessionInfo{} }

func (m *mockSession) Send(payload []byte) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, append([]byte(nil), payload...))
	return nil
}

func (m *mockSession) SendWithContext(_ context.Context, payload []byte) error {
	return m.Send(payload)
}

func (m *mockSession) SendError(payload []byte) error {
	return m.Send(payload)
}

func (m *mockSession) SendErrorWithContext(_ context.Context, payload []byte) error {
	return m.Send(payload)
}

func (m *mockSession) Receive() <-chan *view.Message { return m.recv }

func (m *mockSession) Close() {}

type mockViewContext struct {
	session      view.Session
	getSessionFn func(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error)
	runViewFn    func(v view.View, opts ...view.RunViewOption) (interface{}, error)
	getServiceFn func(v interface{}) (interface{}, error)
	isMeFn       func(id view.Identity) bool
}

func (m *mockViewContext) ID() string        { return "ctx" }
func (m *mockViewContext) Me() view.Identity { return view.Identity("me") }
func (m *mockViewContext) IsMe(id view.Identity) bool {
	if m.isMeFn != nil {
		return m.isMeFn(id)
	}
	return false
}
func (m *mockViewContext) Initiator() view.View { return nil }
func (m *mockViewContext) GetSession(caller view.View, party view.Identity, boundToViews ...view.View) (view.Session, error) {
	if m.getSessionFn != nil {
		return m.getSessionFn(caller, party, boundToViews...)
	}
	return nil, nil
}

func (m *mockViewContext) GetSessionByID(string, view.Identity) (view.Session, error) {
	return nil, nil
}
func (m *mockViewContext) Session() view.Session    { return m.session }
func (m *mockViewContext) Context() context.Context { return context.Background() }
func (m *mockViewContext) RunView(v view.View, opts ...view.RunViewOption) (interface{}, error) {
	if m.runViewFn != nil {
		return m.runViewFn(v, opts...)
	}
	return nil, nil
}
func (m *mockViewContext) OnError(func()) {}
func (m *mockViewContext) GetService(v interface{}) (interface{}, error) {
	if m.getServiceFn != nil {
		return m.getServiceFn(v)
	}
	return nil, nil
}

func (m *mockViewContext) StartSpanFrom(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

type mockCodec struct {
	marshalRaw    []byte
	marshalErr    error
	unmarshalErr  error
	lastUnmarshal []byte
	unmarshalFn   func(raw []byte, v interface{}) error
}

func (m *mockCodec) Marshal(v interface{}) ([]byte, error) {
	if m.marshalErr != nil {
		return nil, m.marshalErr
	}
	return append([]byte(nil), m.marshalRaw...), nil
}

func (m *mockCodec) Unmarshal(raw []byte, v interface{}) error {
	m.lastUnmarshal = append([]byte(nil), raw...)
	if m.unmarshalFn != nil {
		return m.unmarshalFn(raw, v)
	}
	return m.unmarshalErr
}

type mockMarshaller struct {
	raw []byte
	err error
}

func (m *mockMarshaller) Marshal(v interface{}) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return append([]byte(nil), m.raw...), nil
}

type mockVaultService struct{}

func (m *mockVaultService) Vault(string, string) (Vault, error) {
	return nil, nil
}

type mockServiceProvider struct {
	getFn func(v any) (any, error)
}

func (m *mockServiceProvider) GetService(v any) (any, error) {
	return m.getFn(v)
}

func TestGetVaultService(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		p := &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return &mockVaultService{}, nil
			},
		}
		vs, err := GetVaultService(p)
		require.NoError(t, err)
		require.NotNil(t, vs)
	})

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("get service failed")
		p := &mockServiceProvider{
			getFn: func(v any) (any, error) {
				return nil, expected
			},
		}
		_, err := GetVaultService(p)
		require.ErrorIs(t, err, expected)
	})
}

func TestFlowConstructors(t *testing.T) {
	t.Parallel()

	state := &struct{}{}
	rv := NewReceiveView(state)
	require.NotNil(t, rv)
	require.Same(t, state, rv.state)
	require.NotNil(t, rv.unmarshaller)

	pv := NewPayloadReceiveView()
	require.NotNil(t, pv)

	party := view.Identity("bob")
	srv := NewSendReceiveView(map[string]string{"k": "v"}, &struct{}{}, party)
	require.NotNil(t, srv)
	require.Equal(t, party, srv.party)
	require.NotNil(t, srv.coded)

	rply := NewReplyView(state)
	require.NotNil(t, rply)
	require.Same(t, state, rply.state)
	require.NotNil(t, rply.marshaller)
}

func TestReceiveViewCall(t *testing.T) {
	t.Parallel()

	t.Run("error message", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.ERROR, Payload: []byte("remote error")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		rv := NewReceiveView(&struct{}{})
		_, err := rv.Call(ctx)
		require.EqualError(t, err, "remote error")
	})

	t.Run("unmarshal success", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte(`{"k":"v"}`)}
		close(ch)
		dst := &struct {
			K string `json:"k"`
		}{}
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		rv := NewReceiveView(dst)
		res, err := rv.Call(ctx)
		require.NoError(t, err)
		require.Same(t, dst, res)
		require.Equal(t, "v", dst.K)
	})
}

func TestPayloadReceiveViewCall(t *testing.T) {
	t.Parallel()

	t.Run("error message", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.ERROR, Payload: []byte("remote error")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		_, err := NewPayloadReceiveView().Call(ctx)
		require.EqualError(t, err, "remote error")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte("payload")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		raw, err := NewPayloadReceiveView().Call(ctx)
		require.NoError(t, err)
		require.Equal(t, []byte("payload"), raw)
	})
}

func TestSendReceiveViewCall(t *testing.T) {
	t.Parallel()

	party := view.Identity("bob")

	t.Run("get session error", func(t *testing.T) {
		t.Parallel()
		srv := NewSendReceiveView("send", &struct{}{}, party)
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return nil, errors.New("session error")
			},
		}
		_, err := srv.Call(ctx)
		require.EqualError(t, err, "session error")
	})

	t.Run("marshal error", func(t *testing.T) {
		t.Parallel()
		srv := NewSendReceiveView("send", &struct{}{}, party)
		srv.coded = &mockCodec{marshalErr: errors.New("marshal failed")}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: make(chan *view.Message)}, nil
			},
		}
		_, err := srv.Call(ctx)
		require.EqualError(t, err, "marshal failed")
	})

	t.Run("send error", func(t *testing.T) {
		t.Parallel()
		srv := NewSendReceiveView("send", &struct{}{}, party)
		srv.coded = &mockCodec{marshalRaw: []byte("send")}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: make(chan *view.Message), sendErr: errors.New("send failed")}, nil
			},
		}
		_, err := srv.Call(ctx)
		require.EqualError(t, err, "send failed")
	})

	t.Run("receive error message", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.ERROR, Payload: []byte("remote error")}
		close(ch)
		srv := NewSendReceiveView("send", &struct{}{}, party)
		srv.coded = &mockCodec{marshalRaw: []byte("send")}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: ch}, nil
			},
		}
		_, err := srv.Call(ctx)
		require.EqualError(t, err, "remote error")
	})

	t.Run("unmarshal error", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte("payload")}
		close(ch)
		srv := NewSendReceiveView("send", &struct{}{}, party)
		srv.coded = &mockCodec{
			marshalRaw:   []byte("send"),
			unmarshalErr: errors.New("unmarshal failed"),
		}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: ch}, nil
			},
		}
		_, err := srv.Call(ctx)
		require.EqualError(t, err, "unmarshal failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte(`{"v":"ok"}`)}
		close(ch)

		dst := &struct {
			V string `json:"v"`
		}{}
		srv := NewSendReceiveView("send", dst, party)
		srv.coded = &mockCodec{
			marshalRaw: []byte("send"),
			unmarshalFn: func(raw []byte, v interface{}) error {
				require.Equal(t, []byte(`{"v":"ok"}`), raw)
				target := v.(*struct {
					V string `json:"v"`
				})
				target.V = "ok"
				return nil
			},
		}
		ms := &mockSession{recv: ch}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return ms, nil
			},
		}
		res, err := srv.Call(ctx)
		require.NoError(t, err)
		require.Same(t, dst, res)
		require.Equal(t, "ok", dst.V)
		require.Len(t, ms.sent, 1)
		require.Equal(t, []byte("send"), ms.sent[0])
	})
}

func TestReplyViewCall(t *testing.T) {
	t.Parallel()

	t.Run("marshal error", func(t *testing.T) {
		t.Parallel()
		rv := &replyView{state: "state", marshaller: &mockMarshaller{err: errors.New("marshal failed")}}
		_, err := rv.Call(&mockViewContext{session: &mockSession{recv: make(chan *view.Message)}})
		require.EqualError(t, err, "marshal failed")
	})

	t.Run("send error", func(t *testing.T) {
		t.Parallel()
		rv := &replyView{state: "state", marshaller: &mockMarshaller{raw: []byte("payload")}}
		_, err := rv.Call(&mockViewContext{session: &mockSession{recv: make(chan *view.Message), sendErr: errors.New("send failed")}})
		require.EqualError(t, err, "send failed")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		ms := &mockSession{recv: make(chan *view.Message)}
		rv := &replyView{state: "state", marshaller: &mockMarshaller{raw: []byte("payload")}}
		res, err := rv.Call(&mockViewContext{session: ms})
		require.NoError(t, err)
		require.Nil(t, res)
		require.Len(t, ms.sent, 1)
		require.Equal(t, []byte("payload"), ms.sent[0])
	})
}

func TestTransactionViewConstructorsAndHelpers(t *testing.T) {
	t.Parallel()

	party := view.Identity("bob")

	rv := NewReceiveTransactionView()
	require.NotNil(t, rv)
	require.True(t, rv.party.IsNone())

	rvFrom := NewReceiveTransactionFromView(party)
	require.NotNil(t, rvFrom)
	require.True(t, rvFrom.party.Equal(party))

	tx := &Transaction{}
	send := NewSendTransactionView(tx, party)
	require.NotNil(t, send)
	require.Same(t, tx, send.tx)
	require.Equal(t, []view.Identity{party}, send.parties)

	sendBack := NewSendTransactionBackView(tx)
	require.NotNil(t, sendBack)
	require.Same(t, tx, sendBack.tx)
}

func TestReceiveTransactionViews(t *testing.T) {
	t.Parallel()

	t.Run("receiveTransactionView session error payload", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.ERROR, Payload: []byte("remote error")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		_, err := NewReceiveTransactionView().Call(ctx)
		require.EqualError(t, err, "remote error")
	})

	t.Run("receiveTransactionFromView get session error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return nil, errors.New("session error")
			},
		}
		_, err := NewReceiveTransactionFromView(view.Identity("bob")).Call(ctx)
		require.EqualError(t, err, "session error")
	})
}

func TestRunViewWrappers(t *testing.T) {
	t.Parallel()

	tx := &Transaction{}

	t.Run("RequestRecipientIdentity wrapper", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return view.Identity("recipient"), nil
			},
		}
		id, err := RequestRecipientIdentity(ctx, view.Identity("other"))
		require.NoError(t, err)
		require.Equal(t, view.Identity("recipient"), id)
	})

	t.Run("ReceiveTransaction wrapper error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return nil, errors.New("run failed")
			},
		}
		_, err := ReceiveTransaction(ctx)
		require.EqualError(t, err, "run failed")
	})

	t.Run("ReceiveTransaction wrapper success", func(t *testing.T) {
		t.Parallel()
		expected := &Transaction{}
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return expected, nil
			},
		}
		got, err := ReceiveTransaction(ctx)
		require.NoError(t, err)
		require.Same(t, expected, got)
	})

	t.Run("ReceiveTransactionFrom wrapper success", func(t *testing.T) {
		t.Parallel()
		expected := &Transaction{}
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return expected, nil
			},
		}
		got, err := ReceiveTransactionFrom(ctx, view.Identity("bob"))
		require.NoError(t, err)
		require.Same(t, expected, got)
	})

	t.Run("SendAndReceiveTransaction wrapper error on send", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return nil, errors.New("send failed")
			},
		}
		_, err := SendAndReceiveTransaction(ctx, tx, view.Identity("bob"))
		require.EqualError(t, err, "send failed")
	})

	t.Run("SendAndReceiveTransaction wrapper success", func(t *testing.T) {
		t.Parallel()
		expected := &Transaction{}
		callCount := 0
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				callCount++
				if callCount == 1 {
					return nil, nil
				}
				return expected, nil
			},
		}
		got, err := SendAndReceiveTransaction(ctx, tx, view.Identity("bob"))
		require.NoError(t, err)
		require.Same(t, expected, got)
		require.Equal(t, 2, callCount)
	})

	t.Run("SendBackAndReceiveTransaction wrapper error on send", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				return nil, errors.New("send back failed")
			},
		}
		_, err := SendBackAndReceiveTransaction(ctx, tx)
		require.EqualError(t, err, "send back failed")
	})

	t.Run("SendBackAndReceiveTransaction wrapper success", func(t *testing.T) {
		t.Parallel()
		expected := &Transaction{}
		callCount := 0
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				callCount++
				if callCount == 1 {
					return nil, nil
				}
				return expected, nil
			},
		}
		got, err := SendBackAndReceiveTransaction(ctx, tx)
		require.NoError(t, err)
		require.Same(t, expected, got)
		require.Equal(t, 2, callCount)
	})
}

func TestTransactionConstructorsErrorAndWrap(t *testing.T) {
	t.Parallel()

	t.Run("wrap sets default certification", func(t *testing.T) {
		t.Parallel()
		etx := newTestEndorserTransactionWithRWSet(newTestRWSet())
		tx, err := Wrap(etx)
		require.NoError(t, err)
		require.NotNil(t, tx)
		typ, _, err := GetCertificationType(tx.Transaction)
		require.NoError(t, err)
		require.Equal(t, ChaincodeCertification, typ)
	})

	t.Run("new transaction error paths", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getServiceFn: func(v interface{}) (interface{}, error) {
				return nil, errors.New("service missing")
			},
		}

		_, err := NewTransaction(ctx)
		require.Error(t, err)

		_, err = NewAnonymousTransaction(ctx)
		require.Error(t, err)

		_, err = NewTransactionFromBytes(ctx, []byte("raw"))
		require.Error(t, err)
	})
}

func TestSendTransactionViewCallBranches(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ms := &mockSession{recv: make(chan *view.Message)}
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return ms, nil
			},
		}
		_, err := NewSendTransactionView(tx, view.Identity("bob")).Call(ctx)
		require.NoError(t, err)
		require.Len(t, ms.sent, 1)
		require.Equal(t, []byte("tx-bytes"), ms.sent[0])
	})

	t.Run("skip me", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ms := &mockSession{recv: make(chan *view.Message)}
		ctx := &mockViewContext{
			isMeFn: func(id view.Identity) bool {
				return id.Equal(view.Identity("me"))
			},
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return ms, nil
			},
		}
		_, err := NewSendTransactionView(tx, view.Identity("me")).Call(ctx)
		require.NoError(t, err)
		require.Len(t, ms.sent, 0)
	})

	t.Run("bytes error", func(t *testing.T) {
		t.Parallel()
		tx, _, dtx := newTestStateTransaction("assetns")
		dtx.bytesErr = errors.New("marshal failed")
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: make(chan *view.Message)}, nil
			},
		}
		_, err := NewSendTransactionView(tx, view.Identity("bob")).Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed marshalling transaction content")
	})

	t.Run("get session error", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return nil, errors.New("session failed")
			},
		}
		_, err := NewSendTransactionView(tx, view.Identity("bob")).Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting session")
	})

	t.Run("send error", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: make(chan *view.Message), sendErr: errors.New("send failed")}, nil
			},
		}
		_, err := NewSendTransactionView(tx, view.Identity("bob")).Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed sending transaction content")
	})
}

func TestSendTransactionBackViewCallBranches(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ms := &mockSession{recv: make(chan *view.Message)}
		ctx := &mockViewContext{session: ms}
		_, err := NewSendTransactionBackView(tx).Call(ctx)
		require.NoError(t, err)
		require.Len(t, ms.sent, 1)
		require.Equal(t, []byte("tx-bytes"), ms.sent[0])
	})

	t.Run("bytes error", func(t *testing.T) {
		t.Parallel()
		tx, _, dtx := newTestStateTransaction("assetns")
		dtx.bytesErr = errors.New("marshal failed")
		ctx := &mockViewContext{session: &mockSession{recv: make(chan *view.Message)}}
		_, err := NewSendTransactionBackView(tx).Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed marshalling transaction content")
	})

	t.Run("send error", func(t *testing.T) {
		t.Parallel()
		tx, _, _ := newTestStateTransaction("assetns")
		ctx := &mockViewContext{session: &mockSession{recv: make(chan *view.Message), sendErr: errors.New("send failed")}}
		_, err := NewSendTransactionBackView(tx).Call(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed sending transaction content")
	})
}

func TestRecipientViewsAndWrappers(t *testing.T) {
	t.Parallel()

	t.Run("request view get session error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return nil, errors.New("session failed")
			},
		}
		_, err := (&RequestRecipientIdentityView{Other: view.Identity("bob")}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("request view send error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: make(chan *view.Message), sendErr: errors.New("send failed")}, nil
			},
		}
		_, err := (&RequestRecipientIdentityView{Other: view.Identity("bob")}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("request view invalid payload", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte("{bad-json")}
		close(ch)
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return &mockSession{recv: ch}, nil
			},
		}
		_, err := (&RequestRecipientIdentityView{Other: view.Identity("bob")}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("respond request view read error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{session: &mockSession{recv: nil}}
		_, err := (&RespondRequestRecipientIdentityView{}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("respond request view bad payload", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte("{bad-json")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		_, err := (&RespondRequestRecipientIdentityView{}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("exchange view get session error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return nil, errors.New("session failed")
			},
		}
		_, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("bob")}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("respond exchange view read error", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{session: &mockSession{recv: nil}}
		_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("respond exchange view bad payload", func(t *testing.T) {
		t.Parallel()
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: []byte("{bad-json")}
		close(ch)
		ctx := &mockViewContext{session: &mockSession{recv: ch}}
		_, err := (&RespondExchangeRecipientIdentitiesView{}).Call(ctx)
		require.Error(t, err)
	})

	t.Run("recipient wrappers", func(t *testing.T) {
		t.Parallel()
		ctx := &mockViewContext{
			runViewFn: func(v view.View, opts ...view.RunViewOption) (interface{}, error) {
				switch v.(type) {
				case *RespondRequestRecipientIdentityView:
					return view.Identity("me"), nil
				case *ExchangeRecipientIdentitiesView:
					return []view.Identity{view.Identity("me"), view.Identity("other")}, nil
				case *RespondExchangeRecipientIdentitiesView:
					return []view.Identity{view.Identity("me"), view.Identity("other")}, nil
				default:
					return nil, errors.New("unexpected view")
				}
			},
		}

		id, err := RespondRequestRecipientIdentity(ctx)
		require.NoError(t, err)
		require.Equal(t, view.Identity("me"), id)

		me, other, err := ExchangeRecipientIdentities(ctx, view.Identity("other"))
		require.NoError(t, err)
		require.Equal(t, view.Identity("me"), me)
		require.Equal(t, view.Identity("other"), other)

		me, other, err = RespondExchangeRecipientIdentities(ctx)
		require.NoError(t, err)
		require.Equal(t, view.Identity("me"), me)
		require.Equal(t, view.Identity("other"), other)
	})
}

func TestRecipientViewsSuccessfulFlows(t *testing.T) {
	t.Parallel()

	newCtx := func(session view.Session) *mockViewContext {
		bindingStore := newTestBindingStore()
		endpointService, err := endpoint.NewService(bindingStore)
		require.NoError(t, err)

		nsp := newTestFabricNetworkServiceProvider(view.Identity("me"), &mockDriverChannel{
			name:     "ch",
			metadata: &mockDriverMetadataService{},
		})

		return &mockViewContext{
			session: session,
			getSessionFn: func(view.View, view.Identity, ...view.View) (view.Session, error) {
				return session, nil
			},
			getServiceFn: func(v interface{}) (interface{}, error) {
				rt, ok := v.(reflect.Type)
				if !ok {
					return nil, errors.New("service key is not a reflect.Type")
				}
				switch rt {
				case reflect.TypeOf((*fabric.NetworkServiceProvider)(nil)):
					return nsp, nil
				case reflect.TypeOf((*endpoint.Service)(nil)):
					return endpointService, nil
				case reflect.TypeOf((*VaultService)(nil)):
					return &mockVaultService{}, nil
				default:
					return nil, errors.New("service missing")
				}
			},
		}
	}

	t.Run("request recipient identity success", func(t *testing.T) {
		t.Parallel()
		data := &RecipientData{Identity: view.Identity("other-recipient")}
		raw, err := data.Bytes()
		require.NoError(t, err)

		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: raw}
		close(ch)

		session := &mockSession{recv: ch}
		ctx := newCtx(session)

		out, err := (&RequestRecipientIdentityView{Other: view.Identity("other")}).Call(ctx)
		require.NoError(t, err)
		require.Equal(t, view.Identity("other-recipient"), out.(view.Identity))
		require.Len(t, session.sent, 1)
	})

	t.Run("respond request recipient identity success", func(t *testing.T) {
		t.Parallel()
		req := &RecipientRequest{Network: ""}
		reqRaw, err := req.Bytes()
		require.NoError(t, err)

		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: reqRaw}
		close(ch)
		session := &mockSession{recv: ch}

		ctx := newCtx(session)
		out, err := (&RespondRequestRecipientIdentityView{}).Call(ctx)
		require.NoError(t, err)
		require.Equal(t, view.Identity("me"), out.(view.Identity))
		require.Len(t, session.sent, 1)
	})

	t.Run("exchange recipient identities success", func(t *testing.T) {
		t.Parallel()
		data := &RecipientData{Identity: view.Identity("other-recipient")}
		raw, err := data.Bytes()
		require.NoError(t, err)

		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: raw}
		close(ch)
		session := &mockSession{recv: ch}

		ctx := newCtx(session)
		out, err := (&ExchangeRecipientIdentitiesView{Other: view.Identity("other")}).Call(ctx)
		require.NoError(t, err)
		ids := out.([]view.Identity)
		require.Len(t, ids, 2)
		require.Equal(t, view.Identity("me"), ids[0])
		require.Equal(t, view.Identity("other-recipient"), ids[1])
		require.Len(t, session.sent, 1)
	})

	t.Run("respond exchange recipient identities success", func(t *testing.T) {
		t.Parallel()
		req := &ExchangeRecipientRequest{
			RecipientData: &RecipientData{Identity: view.Identity("other-recipient")},
		}
		reqRaw, err := req.Bytes()
		require.NoError(t, err)

		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Status: view.OK, Payload: reqRaw}
		close(ch)
		session := &mockSession{recv: ch}

		ctx := newCtx(session)
		out, err := (&RespondExchangeRecipientIdentitiesView{}).Call(ctx)
		require.NoError(t, err)
		ids := out.([]view.Identity)
		require.Len(t, ids, 2)
		require.Equal(t, view.Identity("me"), ids[0])
		require.Equal(t, view.Identity("other-recipient"), ids[1])
		require.Len(t, session.sent, 1)
	})
}

type mockNetworkForRWSetProcessor struct {
	err error
	ch  *fabric.Channel
}

func (m *mockNetworkForRWSetProcessor) Channel(string) (*fabric.Channel, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ch, nil
}

type mockProcessTransactionForRWSetProcessor struct {
	channel string
	id      string
}

func (m *mockProcessTransactionForRWSetProcessor) Network() string { return "net" }
func (m *mockProcessTransactionForRWSetProcessor) Channel() string {
	if m.channel == "" {
		return "ch"
	}
	return m.channel
}

func (m *mockProcessTransactionForRWSetProcessor) ID() string {
	if m.id == "" {
		return "tx-id"
	}
	return m.id
}

func (m *mockProcessTransactionForRWSetProcessor) FunctionAndParameters() (string, []string) {
	return "fn", nil
}

type mockRequestForRWSetProcessor struct{}

func (m *mockRequestForRWSetProcessor) ID() string { return "req-id" }

func TestRWSetProcessorProcessChannelError(t *testing.T) {
	t.Parallel()

	p := NewRWSetProcessor(&mockNetworkForRWSetProcessor{err: errors.New("channel failed")})
	err := p.Process(&mockRequestForRWSetProcessor{}, &mockProcessTransactionForRWSetProcessor{}, fabric.NewRWSet(newTestRWSet()), "assetns")
	require.Error(t, err)
	require.ErrorContains(t, err, "failed getting channel")
}

type mockDriverMetadataService struct {
	exists       bool
	transientMap fdriver.TransientMap
	loadErr      error
}

func (m *mockDriverMetadataService) Exists(context.Context, string) bool {
	return m.exists
}

func (m *mockDriverMetadataService) StoreTransient(context.Context, string, fdriver.TransientMap) error {
	return nil
}

func (m *mockDriverMetadataService) LoadTransient(context.Context, string) (fdriver.TransientMap, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.transientMap, nil
}

type mockDriverChannel struct {
	name     string
	metadata fdriver.MetadataService
}

func (m *mockDriverChannel) Name() string {
	if m.name == "" {
		return "ch"
	}
	return m.name
}

func (m *mockDriverChannel) Committer() fdriver.Committer                           { return nil }
func (m *mockDriverChannel) Vault() fdriver.Vault                                   { return nil }
func (m *mockDriverChannel) Delivery() fdriver.Delivery                             { return nil }
func (m *mockDriverChannel) Ledger() fdriver.Ledger                                 { return nil }
func (m *mockDriverChannel) Finality() fdriver.Finality                             { return nil }
func (m *mockDriverChannel) ChannelMembership() fdriver.ChannelMembership           { return nil }
func (m *mockDriverChannel) ChaincodeManager() fdriver.ChaincodeManager             { return nil }
func (m *mockDriverChannel) RWSetLoader() fdriver.RWSetLoader                       { return nil }
func (m *mockDriverChannel) EnvelopeService() fdriver.EnvelopeService               { return nil }
func (m *mockDriverChannel) TransactionService() fdriver.EndorserTransactionService { return nil }
func (m *mockDriverChannel) MetadataService() fdriver.MetadataService               { return m.metadata }
func (m *mockDriverChannel) Close() error                                           { return nil }

func TestRWSetProcessorProcessKnownTransaction(t *testing.T) {
	t.Parallel()

	rwset := newTestRWSet()
	key, err := CreateCompositeKey("asset", []string{"1"})
	require.NoError(t, err)
	require.NoError(t, rwset.SetState("assetns", key, []byte("value")))

	k, err := fieldMappingKey("assetns", key)
	require.NoError(t, err)

	ms := &mockDriverMetadataService{
		exists:       true,
		transientMap: fdriver.TransientMap{k: []byte("mapping")},
	}
	ch := fabric.NewChannel(nil, nil, &mockDriverChannel{name: "ch", metadata: ms})
	p := NewRWSetProcessor(&mockNetworkForRWSetProcessor{ch: ch})
	err = p.Process(
		&mockRequestForRWSetProcessor{},
		&mockProcessTransactionForRWSetProcessor{channel: "ch", id: "tx-id"},
		fabric.NewRWSet(rwset),
		"assetns",
	)
	require.NoError(t, err)

	meta, err := rwset.GetStateMetadata("assetns", key)
	require.NoError(t, err)
	require.Equal(t, []byte("mapping"), meta[k])
}

func TestRWSetProcessorProcessUnknownTransaction(t *testing.T) {
	t.Parallel()

	rwset := newTestRWSet()
	key, err := CreateCompositeKey("asset", []string{"1"})
	require.NoError(t, err)
	require.NoError(t, rwset.SetState("assetns", key, []byte("value")))

	ms := &mockDriverMetadataService{exists: false}
	ch := fabric.NewChannel(nil, nil, &mockDriverChannel{name: "ch", metadata: ms})
	p := NewRWSetProcessor(&mockNetworkForRWSetProcessor{ch: ch})
	err = p.Process(
		&mockRequestForRWSetProcessor{},
		&mockProcessTransactionForRWSetProcessor{channel: "ch", id: "tx-id"},
		fabric.NewRWSet(rwset),
		"assetns",
	)
	require.NoError(t, err)
}

var _ services.Provider = &mockServiceProvider{}
