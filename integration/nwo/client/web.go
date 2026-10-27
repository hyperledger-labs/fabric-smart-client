/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"io"
	"net/http"

	dto "github.com/prometheus/client_model/go"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
)

// WebClient calls an FSC node's views through the view service's HTTP binding, reads the
// node's metrics from its operations endpoint, and probes its readiness.
type WebClient struct {
	view    *viewclient.Client
	metrics *MetricsClient
	web     *viewclient.HTTPClient
}

// NewWebClient returns a WebClient for the FSC node whose configuration directory is
// confDir. Its view calls are signed by the node's own identity (fsc.identity), which the
// node always authorizes, and present the node's TLS certificate.
func NewWebClient(confDir string, tracerProvider tracing.Provider) (*WebClient, error) {
	mc, err := NewMetricsClientConfigFromFSC(confDir)
	if err != nil {
		return nil, err
	}
	metrics, err := NewMetricsClient(mc)
	if err != nil {
		return nil, err
	}
	view, err := NewHTTPViewClient(confDir, tracerProvider)
	if err != nil {
		return nil, err
	}
	cc, err := NewWebConnectionConfigFromFSC(confDir)
	if err != nil {
		return nil, err
	}
	web, err := viewclient.NewHTTPClient(cc)
	if err != nil {
		return nil, err
	}
	return &WebClient{view: view, metrics: metrics, web: web}, nil
}

// NewHTTPViewClient returns a view client that reaches the view service of the FSC node
// whose configuration directory is confDir over HTTP, signed by the node's own identity
// (fsc.identity) and presenting the node's TLS certificate.
func NewHTTPViewClient(confDir string, tracerProvider tracing.Provider) (*viewclient.Client, error) {
	cc, err := NewWebConnectionConfigFromFSC(confDir)
	if err != nil {
		return nil, err
	}
	configProvider, err := config.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	signer, err := viewclient.NewX509SigningIdentity(
		configProvider.GetPath("fsc.identity.cert.file"), configProvider.GetPath("fsc.identity.key.file"))
	if err != nil {
		return nil, errors.WithMessage(err, "failed loading fsc.identity")
	}
	return viewclient.NewClient(&viewclient.Config{ConnectionConfig: cc, Transport: viewclient.TransportHTTP}, signer, tracerProvider)
}

// CallView calls view fid on input in through the view service.
func (c *WebClient) CallView(fid string, in []byte) (any, error) { return c.view.CallView(fid, in) }

// CallViewWithContext calls view fid on input in through the view service, bounded by ctx.
func (c *WebClient) CallViewWithContext(ctx context.Context, fid string, in []byte) (any, error) {
	return c.view.CallViewWithContext(ctx, fid, in)
}

// StreamCallView calls view fid on input in and returns a stream to communicate with it.
func (c *WebClient) StreamCallView(fid string, in []byte) (*viewclient.Stream, error) {
	return c.view.StreamCallView(fid, in)
}

// Metrics returns the metric families the node exposes on its operations endpoint.
func (c *WebClient) Metrics() (map[string]*dto.MetricFamily, error) { return c.metrics.Metrics() }

// Ready returns nil when the node's GET /readyz answers 200, and an error quoting the
// unready components otherwise.
func (c *WebClient) Ready() error {
	url := c.web.URL + "/readyz"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrapf(err, "failed creating request to [%s]", url)
	}
	resp, err := c.web.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed calling [%s]", url)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxMetricsErrorBody))
		return errors.Errorf("[%s] answered [%s]: %s", url, resp.Status, msg)
	}
	return nil
}

// NewWebConnectionConfigFromFSC returns the connection to an FSC node's web listener, read
// from the node's configuration directory.
//
// The node's own TLS material is resolved through tlsconfig, so it is inherited from fsc.tls
// exactly as the listener inherits it. Reading fsc.web.tls.enabled directly would report
// false for a node that inherits the field, and the client would then dial plaintext against
// a TLS listener.
func NewWebConnectionConfigFromFSC(confDir string) (*grpc.ConnectionConfig, error) {
	configProvider, err := config.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	tlsOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.web.tls")
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving fsc.web.tls")
	}
	return selfConnection(configProvider.GetString("fsc.web.address"), tlsOpts)
}
