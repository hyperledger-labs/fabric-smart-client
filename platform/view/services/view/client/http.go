/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	commongrpc "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	webclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/wsproto"
)

// idleConnTimeout is how long an idle pooled connection to the view service is kept.
const idleConnTimeout = 90 * time.Second

// maxErrorBody bounds the body of a non-200 response quoted in an error.
const maxErrorBody = 4 << 10

// httpTransport reaches the view service's HTTP binding (protos.ProcessCommandHTTPPath). It
// is its own protos.ViewServiceClient: ProcessCommand is a protojson POST, StreamCommand a
// WebSocket of wsproto frames. gRPC call options do not apply and are ignored.
type httpTransport struct {
	url, wsURL     string
	client         *http.Client
	tlsConfig      *tls.Config
	cert           *tls.Certificate
	maxRecvMsgSize int
}

// HTTPClient is an HTTP client for one node listener, built from a ConnectionConfig the same
// way the gRPC client is: it presents the TLS keypair only when cc.TLS.RequireClientCert is
// set, and ConnectionTimeout bounds dialing and the TLS handshake.
type HTTPClient struct {
	*http.Client
	// URL is the listener's base URL, http:// or https:// as cc.TLS selects.
	URL string

	tlsConfig *tls.Config      // nil without TLS
	cert      *tls.Certificate // the client certificate presented, nil when none is
}

// NewHTTPClient returns an HTTPClient for the listener cc describes. It returns an error when
// mutual TLS is required without a keypair, or when the TLS material does not load.
func NewHTTPClient(cc *commongrpc.ConnectionConfig) (*HTTPClient, error) {
	opts := cc.TLS
	if opts.RequireClientCert && (len(opts.Certificate) == 0 || len(opts.Key) == 0) {
		return nil, errors.New("both Key and Certificate are required when using mutual TLS")
	}
	if !opts.RequireClientCert {
		opts.Certificate, opts.Key = nil, nil
	}
	tlsConfig, err := opts.TLSConfig()
	if err != nil {
		return nil, errors.WithMessage(err, "failed building client TLS")
	}
	timeout := cmp.Or(cc.ConnectionTimeout, commongrpc.DefaultConnectionTimeout)
	c := &HTTPClient{
		Client: &http.Client{Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: tlsConfig,
			// The connection timeout bounds dialing and the TLS handshake, as on gRPC.
			DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
			TLSHandshakeTimeout: timeout,
			IdleConnTimeout:     idleConnTimeout,
		})},
		URL:       "http://" + cc.Address,
		tlsConfig: tlsConfig,
	}
	if tlsConfig != nil {
		c.URL = "https://" + cc.Address
		if len(tlsConfig.Certificates) > 0 {
			c.cert = &tlsConfig.Certificates[0]
		}
	}
	return c, nil
}

func newHTTPTransport(cc *commongrpc.ConnectionConfig) (*httpTransport, error) {
	// The same client rules as the gRPC transport, so both present the same certificate.
	c, err := NewHTTPClient(cc)
	if err != nil {
		return nil, err
	}
	return &httpTransport{
		url:            c.URL,
		wsURL:          "ws" + strings.TrimPrefix(c.URL, "http"),
		client:         c.Client,
		tlsConfig:      c.tlsConfig,
		cert:           c.cert,
		maxRecvMsgSize: cmp.Or(cc.MaxRecvMsgSize, protos.DefaultHTTPMaxRecvMsgSize),
	}, nil
}

// Open returns t itself: the HTTP client pools connections, so there is nothing to release.
func (t *httpTransport) Open() (protos.ViewServiceClient, io.Closer, error) {
	return t, io.NopCloser(nil), nil
}

func (t *httpTransport) Certificate() *tls.Certificate { return t.cert }

// ProcessCommand posts sc to the view service. A 503 answer (a draining node) becomes a gRPC
// Unavailable status and a 409 answer (a view the node's shutdown cancelled) an Aborted one,
// as the gRPC transport would report them.
func (t *httpTransport) ProcessCommand(ctx context.Context, sc *protos.SignedCommand, _ ...grpc.CallOption) (*protos.SignedCommandResponse, error) {
	body, err := protojson.Marshal(sc)
	if err != nil {
		return nil, errors.Wrap(err, "failed encoding command")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url+protos.ProcessCommandHTTPPath, bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "failed creating request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling [%s]", req.URL)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		err := errors.Errorf("view service at [%s] answered [%s]: %s", req.URL, resp.Status, bytes.TrimSpace(msg))
		switch resp.StatusCode {
		case http.StatusServiceUnavailable:
			return nil, status.Error(codes.Unavailable, err.Error())
		case http.StatusConflict:
			return nil, status.Error(codes.Aborted, err.Error())
		}
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(t.maxRecvMsgSize)+1))
	if err != nil {
		return nil, errors.Wrap(err, "failed reading response")
	}
	if len(raw) > t.maxRecvMsgSize {
		return nil, errors.Errorf("response larger than [%d] bytes", t.maxRecvMsgSize)
	}
	scr := &protos.SignedCommandResponse{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, scr); err != nil {
		return nil, errors.Wrap(err, "response is not a SignedCommandResponse")
	}
	return scr, nil
}

func (t *httpTransport) StreamCommand(ctx context.Context, _ ...grpc.CallOption) (protos.ViewService_StreamCommandClient, error) {
	conn, err := webclient.OpenWSClientConnContextWithLimit(ctx, t.wsURL+protos.StreamCommandHTTPPath, t.tlsConfig, int64(t.maxRecvMsgSize))
	if err != nil {
		return nil, errors.Wrap(err, "failed opening view service stream")
	}
	return &wsCommandClient{conn: conn, ctx: ctx}, nil
}

// wsCommandClient is a protos.ViewService_StreamCommandClient over a WebSocket. Headers and
// trailers have no WebSocket equivalent and are empty.
type wsCommandClient struct {
	conn *websocket.Conn
	ctx  context.Context
}

func (c *wsCommandClient) Send(m *protos.SignedCommand) error { return wsproto.Write(c.conn, m) }

func (c *wsCommandClient) Recv() (*protos.SignedCommandResponse, error) {
	m := &protos.SignedCommandResponse{}
	if err := c.read(m); err != nil {
		return nil, err
	}
	return m, nil
}

// read reads m from the stream. A close by a node that drains (code 1013) becomes a gRPC
// Unavailable status, and a close naming a view the node's shutdown cancelled an Aborted one,
// as the gRPC transport reports them.
func (c *wsCommandClient) read(m proto.Message) error {
	err := wsproto.Read(c.conn, m)
	var ce *websocket.CloseError
	if !stderrors.As(err, &ce) {
		return err
	}
	switch {
	case ce.Code == websocket.CloseTryAgainLater:
		return status.Error(codes.Unavailable, ce.Text)
	case ce.Code == websocket.CloseInternalServerErr && ce.Text == protos.AbortedByShutdownMessage:
		return status.Error(codes.Aborted, ce.Text)
	}
	return err
}

func (c *wsCommandClient) SendMsg(m any) error {
	pm, ok := m.(proto.Message)
	if !ok {
		return errors.Errorf("cannot send [%T]: not a protobuf message", m)
	}
	return wsproto.Write(c.conn, pm)
}

func (c *wsCommandClient) RecvMsg(m any) error {
	pm, ok := m.(proto.Message)
	if !ok {
		return errors.Errorf("cannot receive into [%T]: not a protobuf message", m)
	}
	return c.read(pm)
}

func (*wsCommandClient) Header() (metadata.MD, error) { return metadata.MD{}, nil }
func (*wsCommandClient) Trailer() metadata.MD         { return metadata.MD{} }
func (c *wsCommandClient) Context() context.Context   { return c.ctx }

// CloseSend ends the stream in both directions: it sends a close frame and closes the
// connection. The client calls it only once it has received the final response.
func (c *wsCommandClient) CloseSend() error {
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	return c.conn.Close()
}
