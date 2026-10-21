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
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
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

func newHTTPTransport(cc *grpc2.ConnectionConfig) (*httpTransport, error) {
	// A client presents its keypair only when mutual TLS is asked for, as the gRPC client
	// does (grpc.Client), so both transports present the same certificate.
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
	timeout := cmp.Or(cc.ConnectionTimeout, grpc2.DefaultConnectionTimeout)
	scheme, wsScheme := "http", "ws"
	var cert *tls.Certificate
	if tlsConfig != nil {
		scheme, wsScheme = "https", "wss"
		if len(tlsConfig.Certificates) > 0 {
			cert = &tlsConfig.Certificates[0]
		}
	}
	return &httpTransport{
		url:   scheme + "://" + cc.Address,
		wsURL: wsScheme + "://" + cc.Address,
		client: &http.Client{Transport: otelhttp.NewTransport(&http.Transport{
			TLSClientConfig: tlsConfig,
			// The connection timeout bounds dialing and the TLS handshake, as on gRPC.
			DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
			TLSHandshakeTimeout: timeout,
			IdleConnTimeout:     idleConnTimeout,
		})},
		tlsConfig:      tlsConfig,
		cert:           cert,
		maxRecvMsgSize: cmp.Or(cc.MaxRecvMsgSize, protos.DefaultHTTPMaxRecvMsgSize),
	}, nil
}

// Open returns t itself: the HTTP client pools connections, so there is nothing to release.
func (t *httpTransport) Open() (protos.ViewServiceClient, io.Closer, error) {
	return t, io.NopCloser(nil), nil
}

func (t *httpTransport) Certificate() *tls.Certificate { return t.cert }

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
		return nil, errors.Errorf("view service at [%s] answered [%s]: %s", req.URL, resp.Status, bytes.TrimSpace(msg))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(t.maxRecvMsgSize)+1))
	if err != nil {
		return nil, errors.Wrap(err, "failed reading response")
	}
	if len(raw) > t.maxRecvMsgSize {
		return nil, errors.Errorf("response larger than [%d] bytes", t.maxRecvMsgSize)
	}
	scr := &protos.SignedCommandResponse{}
	if err := protojson.Unmarshal(raw, scr); err != nil {
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
	if err := wsproto.Read(c.conn, m); err != nil {
		return nil, err
	}
	return m, nil
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
	return wsproto.Read(c.conn, pm)
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
