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
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
)

// maxMetricsErrorBody bounds the body of a non-200 response quoted in an error.
const maxMetricsErrorBody = 4 << 10

// MetricsClient reads an FSC node's metrics from its operations endpoint (/metrics). It
// reaches the listener over HTTP the way the view client's HTTP transport does, from a
// grpc.ConnectionConfig.
type MetricsClient struct{ http *viewclient.HTTPClient }

// NewMetricsClient returns a MetricsClient for the listener cc describes.
func NewMetricsClient(cc *grpc.ConnectionConfig) (*MetricsClient, error) {
	c, err := viewclient.NewHTTPClient(cc)
	if err != nil {
		return nil, err
	}
	return &MetricsClient{http: c}, nil
}

// Metrics returns the metric families the node exposes.
func (c *MetricsClient) Metrics() (map[string]*dto.MetricFamily, error) {
	url := c.http.URL + "/metrics"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating request to [%s]", url)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling [%s]", url)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxMetricsErrorBody))
		return nil, errors.Errorf("[%s] answered [%s]: %s", url, resp.Status, msg)
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing metrics from [%s]", url)
	}
	return families, nil
}

// NewMetricsClientConfigFromFSC locates an FSC node's metrics endpoint the way the node
// serves it, read from the node's configuration directory: on fsc.metrics.address with
// fsc.metrics.tls when the address is set, and on fsc.web otherwise. TLS is resolved through
// tlsconfig, so it is inherited from fsc.tls exactly as the listener inherits it. The
// listener's server certificate doubles as the trust anchor and as the client certificate;
// the endpoint requires the latter unless fsc.metrics.clientAuthRequired is false. On a
// listener of its own, that certificate must chain to the listener's clientRootCAs and
// allow client authentication.
func NewMetricsClientConfigFromFSC(confDir string) (*grpc.ConnectionConfig, error) {
	configProvider, err := config.NewProvider(confDir)
	if err != nil {
		return nil, err
	}
	addr, key := configProvider.GetString("fsc.metrics.address"), "fsc.metrics.tls"
	if addr == "" {
		addr, key = configProvider.GetString("fsc.web.address"), "fsc.web.tls"
	}
	tlsOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", key)
	if err != nil {
		return nil, errors.Wrapf(err, "failed resolving %s", key)
	}
	return selfConnection(addr, tlsOpts)
}

// selfConnection turns a node listener's server TLS into the client connection the node's
// own tooling uses to reach it: the server certificate doubles as the trust anchor and as
// the client certificate.
func selfConnection(addr string, server grpc.SecureOptions) (*grpc.ConnectionConfig, error) {
	cc := &grpc.ConnectionConfig{Address: addr}
	if !server.UseTLS {
		return cc, nil
	}
	if len(server.Certificate) == 0 {
		return nil, errors.New("TLS is enabled but no certificate is configured")
	}
	cc.TLS = grpc.SecureOptions{
		UseTLS:            true,
		ServerRootCAs:     [][]byte{server.Certificate},
		RequireClientCert: true,
		Certificate:       server.Certificate,
		Key:               server.Key,
	}
	return cc, nil
}
