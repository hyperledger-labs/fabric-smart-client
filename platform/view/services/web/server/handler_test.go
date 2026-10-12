/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/mock"
)

type Fruit struct {
	Name     string
	Quantity int
}

type FruitBasket struct {
	Fruits []string
}

func TestHttpHandler(t *testing.T) {
	t.Parallel()
	h := server.NewHttpHandler()

	rh := &mock.RequestHandler{}
	rh.HandleRequestStub = func(ctx *server.ReqContext) (any, int) {
		query := ctx.Query.(*Fruit)

		var res FruitBasket
		for i := 0; i < query.Quantity; i++ {
			res.Fruits = append(res.Fruits, query.Name)
		}

		require.Equal(t, "pineapple", ctx.Req.PathValue("Fruit"))

		return res, 200
	}

	rh.ParsePayloadStub = func(payload []byte) (any, error) {
		var f Fruit
		err := json.Unmarshal(payload, &f)
		require.NoError(t, err)
		return &f, nil
	}

	h.RegisterURI("/test/{Fruit}", "PUT", rh)

	resp := httptest.NewRecorder()
	pineappleRequest := bytes.NewBufferString(`{"Name": "pineapple", "Quantity": 3}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/test/pineapple", pineappleRequest)
	h.ServeHTTP(resp, req)

	expectedPineappleResponse := FruitBasket{Fruits: []string{"pineapple", "pineapple", "pineapple"}}
	var actualResponse FruitBasket
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &actualResponse))
	require.Equal(t, expectedPineappleResponse, actualResponse)
}

func TestHttpHandlerErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		accept     string
		body       io.Reader
		setup      func(rh *mock.RequestHandler)
		wantCode   int
		wantReason string
		// hidden is internal error text that must not reach the client.
		hidden      string
		wantParsed  int
		wantHandled int
	}{
		{
			name:       "bad accept header",
			accept:     "text/html",
			wantCode:   http.StatusBadRequest,
			wantReason: "bad content type",
		},
		{
			name:       "unparsable accept header",
			accept:     "application/jsonx",
			wantCode:   http.StatusBadRequest,
			wantReason: "bad content type",
		},
		{
			name:       "body too large",
			body:       strings.NewReader(strings.Repeat("a", 10*1024*1024+1)),
			wantCode:   http.StatusRequestEntityTooLarge,
			wantReason: "request too large",
		},
		{
			name:       "unreadable body",
			body:       iotest.ErrReader(errors.New("boom")),
			wantCode:   http.StatusBadRequest,
			wantReason: "failed reading request",
			hidden:     "boom",
		},
		{
			name:       "parse failure",
			setup:      func(rh *mock.RequestHandler) { rh.ParsePayloadReturns(nil, errors.New("bad json")) },
			wantCode:   http.StatusBadRequest,
			wantReason: "failed parsing request",
			hidden:     "bad json",
			wantParsed: 1,
		},
		{
			name:        "encoding failure",
			setup:       func(rh *mock.RequestHandler) { rh.HandleRequestReturns(make(chan int), http.StatusOK) },
			wantCode:    http.StatusInternalServerError,
			wantReason:  "failed encoding response from backend",
			hidden:      "chan",
			wantParsed:  1,
			wantHandled: 1,
		},
		{
			name:        "backend invalid status code",
			setup:       func(rh *mock.RequestHandler) { rh.HandleRequestReturns("x", 0) },
			wantCode:    http.StatusInternalServerError,
			wantReason:  "invalid status code from backend",
			wantParsed:  1,
			wantHandled: 1,
		},
		{
			name:        "backend non-2xx",
			setup:       func(rh *mock.RequestHandler) { rh.HandleRequestReturns("not found", http.StatusNotFound) },
			wantCode:    http.StatusNotFound,
			wantReason:  "\"not found\"\n",
			wantParsed:  1,
			wantHandled: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rh := &mock.RequestHandler{}
			if tc.setup != nil {
				tc.setup(rh)
			}
			h := server.NewHttpHandler()
			h.RegisterURI("/x", http.MethodPost, rh)

			body := tc.body
			if body == nil {
				body = strings.NewReader("{}")
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/x", body)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, tc.wantCode, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			if tc.hidden != "" {
				assert.NotContains(t, rec.Body.String(), tc.hidden)
			}
			var resp server.ResponseErr
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantReason, resp.Reason)
			assert.Equal(t, tc.wantParsed, rh.ParsePayloadCallCount())
			assert.Equal(t, tc.wantHandled, rh.HandleRequestCallCount())
		})
	}
}

func TestHttpHandlerAcceptedContentTypes(t *testing.T) {
	t.Parallel()

	for _, accept := range []string{"", "application/json", "application/*", "*/*", "text/html, application/json", "application/json; charset=utf-8", "text/html;q=0.9, */*;q=0.8"} {
		t.Run(accept, func(t *testing.T) {
			t.Parallel()
			rh := &mock.RequestHandler{}
			rh.HandleRequestReturns("ok", http.StatusOK)
			h := server.NewHttpHandler()
			h.RegisterURI("/x", http.MethodPost, rh)

			req := httptest.NewRequest(http.MethodPost, "/v1/x", strings.NewReader("{}"))
			if accept != "" {
				req.Header.Set("Accept", accept)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, "\"ok\"\n", rec.Body.String())
			assert.Equal(t, 1, rh.HandleRequestCallCount())
		})
	}
}

func TestHttpHandlerWebSocketWritesNoBody(t *testing.T) {
	t.Parallel()

	// Upgrade and Connection values are case-insensitive tokens (RFC 6455, RFC 9110).
	for _, upgrade := range []string{"websocket", "WebSocket"} {
		t.Run(upgrade, func(t *testing.T) {
			t.Parallel()
			rh := &mock.RequestHandler{}
			rh.HandleRequestReturns("ignored", http.StatusOK)
			h := server.NewHttpHandler()
			h.RegisterURI("/x", http.MethodPost, rh)

			req := httptest.NewRequest(http.MethodPost, "/v1/x", strings.NewReader("{}"))
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", upgrade)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			assert.Equal(t, 1, rh.HandleRequestCallCount())
			assert.Empty(t, rec.Body.String())
			assert.Empty(t, rec.Header().Get("Content-Type"))
		})
	}
}

func TestHttpHandlerRouting(t *testing.T) {
	t.Parallel()
	rh := &mock.RequestHandler{}
	h := server.NewHttpHandler()
	h.RegisterURI("/x", http.MethodPost, rh)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	assert.Equal(t, 0, rh.ParsePayloadCallCount())
}

func TestDummyServer(t *testing.T) {
	t.Parallel()
	s := server.NewDummyServer()
	require.NotNil(t, s)
	s.RegisterHandler("/x", http.NotFoundHandler(), true)
	require.NoError(t, s.Start())
	require.NoError(t, s.Stop())
}

func TestNewWSStreamUpgradeFailure(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	stream, err := server.NewWSStream(rec, httptest.NewRequest(http.MethodGet, "/ws", nil))
	require.Error(t, err)
	assert.Nil(t, stream)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestWSStreamRoundTrip(t *testing.T) {
	t.Parallel()

	type result struct {
		closeErr error
		err      error
	}
	done := make(chan result, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := server.NewWSStream(w, r)
		if err != nil {
			done <- result{err: err}
			return
		}
		raw, err := stream.ReadInput()
		if err == nil {
			err = stream.WriteResult(raw)
		}
		done <- result{err: err, closeErr: stream.Close()}
	}))
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"Raw":"aGk="}`)))
	var out server.Output
	require.NoError(t, conn.ReadJSON(&out))
	assert.Equal(t, server.Output{Raw: []byte("hi")}, out)
	_, _, err = conn.ReadMessage()
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)

	r := <-done
	require.NoError(t, r.err)
	require.NoError(t, r.closeErr)
}

// Input the server never read must not turn its close into a TCP reset, which would discard
// output still in flight.
func TestWSStreamCloseDrainsUnreadInput(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := server.NewWSStream(w, r)
		if err != nil {
			done <- err
			return
		}
		_, err = stream.ReadInput()
		if err == nil {
			err = stream.WriteResult([]byte("hi"))
		}
		done <- errors.Join(err, stream.Close())
	}))
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	defer func() { _ = conn.Close() }()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"Raw":"aGk="}`)))
	// Larger than the server's read buffer, so it stays queued in the kernel.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, make([]byte, 64*1024)))
	var out server.Output
	require.NoError(t, conn.ReadJSON(&out))
	_, _, err = conn.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)

	// FIN, not RST.
	_, err = conn.NetConn().Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	require.NoError(t, <-done)
}

// A client that keeps the stream open after reading the result never answers the close frame,
// so Close must give up on the drain quickly.
func TestWSStreamCloseBoundsDrain(t *testing.T) {
	t.Parallel()

	done := make(chan time.Duration, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := server.NewWSStream(w, r)
		if !assert.NoError(t, err) {
			return
		}
		raw, err := stream.ReadInput()
		assert.NoError(t, err)
		assert.NoError(t, stream.WriteResult(raw))
		start := time.Now()
		assert.NoError(t, stream.Close())
		done <- time.Since(start)
	}))
	defer srv.Close()

	c, err := client.NewWSStream("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	require.NoError(t, c.SendInput([]byte("hi")))
	res, err := c.Result()
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), res)
	assert.Less(t, <-done, 500*time.Millisecond)
}

func TestWSStreamCloseWithError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, cause, reason string
	}{
		{name: "short", cause: "view failed", reason: "view failed"},
		// 200 bytes of 2-byte runes: cut to 122 bytes, the last rune boundary within 123.
		{name: "truncated on rune boundary", cause: strings.Repeat("é", 100), reason: strings.Repeat("é", 61)},
		// A view id from the request path can carry bytes that are not UTF-8; they are dropped.
		{name: "invalid utf8", cause: "failed instantiating view [\xff\xfe]: boom", reason: "failed instantiating view []: boom"},
		{name: "invalid utf8 truncated", cause: "view [v\xff" + strings.Repeat("a", 200), reason: "view [v" + strings.Repeat("a", 116)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			done := make(chan error, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				stream, err := server.NewWSStream(w, r)
				if err != nil {
					done <- err
					return
				}
				done <- stream.CloseWithError(errors.New(tc.cause))
			}))
			defer srv.Close()

			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			defer func() { _ = conn.Close() }()

			_, _, err = conn.ReadMessage()
			var closeErr *websocket.CloseError
			require.ErrorAs(t, err, &closeErr)
			assert.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
			assert.Equal(t, tc.reason, closeErr.Text)
			require.NoError(t, <-done)
		})
	}
}

func TestWSStreamReadReportsClosedConnection(t *testing.T) {
	t.Parallel()

	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stream, err := server.NewWSStream(w, r)
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = stream.Close() }()
		_, err = stream.ReadInput()
		done <- err
	}))
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.NoError(t, conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")))
	_ = conn.Close()

	err = <-done
	require.Error(t, err)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}
