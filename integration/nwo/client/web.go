/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"

	dto "github.com/prometheus/client_model/go"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
)

// WebClient calls an FSC node's views through the view service's HTTP binding and reads the
// node's metrics from its web listener.
type WebClient struct {
	view    *viewclient.Client
	metrics *client.Client
}

// NewWebClient returns a WebClient for the FSC node whose configuration directory is
// confDir. Its view calls are signed by the node's own identity (fsc.identity), which the
// node always authorizes, and present the node's TLS certificate.
func NewWebClient(confDir string, tracerProvider tracing.Provider) (*WebClient, error) {
	cfg, err := NewWebClientConfigFromFSC(confDir)
	if err != nil {
		return nil, err
	}
	metrics, err := client.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	view, err := NewHTTPViewClient(confDir, tracerProvider)
	if err != nil {
		return nil, err
	}
	return &WebClient{view: view, metrics: metrics}, nil
}

// NewHTTPViewClient returns a view client that reaches the view service of the FSC node
// whose configuration directory is confDir over HTTP, signed by the node's own identity
// (fsc.identity) and presenting the node's TLS certificate.
func NewHTTPViewClient(confDir string, tracerProvider tracing.Provider) (*viewclient.Client, error) {
	cfg, err := NewWebClientConfigFromFSC(confDir)
	if err != nil {
		return nil, err
	}
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	signer, err := viewclient.NewX509SigningIdentity(
		configProvider.GetPath("fsc.identity.cert.file"), configProvider.GetPath("fsc.identity.key.file"))
	if err != nil {
		return nil, errors.WithMessage(err, "failed loading fsc.identity")
	}
	cc := &grpc.ConnectionConfig{Address: cfg.Host}
	if len(cfg.CACertRaw) > 0 {
		cc.TLS = grpc.SecureOptions{
			UseTLS:            true,
			ServerRootCAs:     [][]byte{cfg.CACertRaw},
			RequireClientCert: len(cfg.TLSCertRaw) > 0,
			Certificate:       cfg.TLSCertRaw,
			Key:               cfg.TLSKeyRaw,
		}
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

// Metrics returns the metric families the node exposes on its web listener.
func (c *WebClient) Metrics() (map[string]*dto.MetricFamily, error) { return c.metrics.Metrics() }

// NewWebClientConfigFromFSC returns the configuration that reaches an FSC node's web listener,
// read from the node's configuration directory.
//
// The node's own TLS material is resolved through tlsconfig, so it is inherited from fsc.tls
// exactly as the listener inherits it. Reading fsc.web.tls.enabled directly would report
// false for a node that inherits the field, and the client would then dial plaintext against
// a TLS listener.
func NewWebClientConfigFromFSC(confDir string) (*client.Config, error) {
	configProvider, err := config2.NewProvider(confDir)
	if err != nil {
		return nil, err
	}

	tlsOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.web.tls")
	if err != nil {
		return nil, errors.Wrap(err, "failed resolving fsc.web.tls")
	}

	config := &client.Config{Host: configProvider.GetString("fsc.web.address")}
	if !tlsOpts.UseTLS {
		return config, nil
	}
	if len(tlsOpts.Certificate) == 0 {
		return nil, errors.New("web configuration has TLS enabled but no certificate")
	}
	// The node's server certificate doubles as the trust anchor and as the client
	// certificate.
	config.CACertRaw = tlsOpts.Certificate
	config.TLSCertRaw = tlsOpts.Certificate
	config.TLSKeyRaw = tlsOpts.Key
	return config, nil
}
