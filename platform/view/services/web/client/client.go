/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

// maxErrorBodySize bounds how much of a non-200 response body is read for its error reason.
const maxErrorBodySize = 64 << 10

// Config models the configuration for the web client
type Config struct {
	// Host to connect to
	Host string
	// CACertRaw is the certificate authority's certificates
	CACertRaw []byte
	// CACertPath is the Certificate Authority Cert Path
	CACertPath string
	// TLSCertRaw is the PEM-encoded TLS client certificate. Callers resolve their TLS
	// material through tlsconfig and pass the PEM directly, so there is no path form.
	TLSCertRaw []byte
	// TLSKeyRaw is the PEM-encoded TLS client key, paired with TLSCertRaw.
	TLSKeyRaw []byte
}

func (c *Config) WsURL() string {
	return c.url("ws")
}

func (c *Config) WebURL() string {
	return c.url("http")
}

func (c *Config) url(protocol string) string {
	if c.isTLSEnabled() {
		protocol += "s"
	}
	return fmt.Sprintf("%s://%s", protocol, c.Host)
}

func (c *Config) isTLSEnabled() bool {
	return c.CACertPath != "" || len(c.CACertRaw) != 0
}

// Client reads a node's metrics from its operations endpoint over HTTP.
type Client struct {
	c             *http.Client
	url           string
	tlsConfig     *tls.Config // the TLS c dials with, nil without TLS
	metricsParser expfmt.TextParser
}

// NewClient returns a client for the listener at config.Host. It returns an error if config
// is nil or its TLS material does not load.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, errors.New("missing client config")
	}

	var tlsClientConfig *tls.Config

	tlsEnabled := len(config.CACertPath) != 0 || len(config.CACertRaw) != 0

	if tlsEnabled {
		rootCAs := x509.NewCertPool()

		caCert := config.CACertRaw
		if len(config.CACertPath) != 0 {
			var err error
			caCert, err = os.ReadFile(config.CACertPath)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to open ca cert")
			}
		}
		rootCAs.AppendCertsFromPEM(caCert)
		tlsClientConfig = &tls.Config{
			RootCAs: rootCAs,
		}

		if len(config.TLSCertRaw) != 0 {
			clientCert, err := tls.X509KeyPair(config.TLSCertRaw, config.TLSKeyRaw)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load x509 key pair")
			}
			tlsClientConfig.Certificates = []tls.Certificate{clientCert}
		}
	}

	return &Client{
		c: &http.Client{
			Transport: otelhttp.NewTransport(&http.Transport{
				TLSClientConfig: tlsClientConfig,
			}),
		},
		url:           config.WebURL(),
		tlsConfig:     tlsClientConfig,
		metricsParser: expfmt.NewTextParser(model.LegacyValidation),
	}, nil
}

// Metrics returns the metric families the node exposes on /metrics.
func (c *Client) Metrics() (map[string]*dto.MetricFamily, error) {
	body, err := c.get(context.Background(), c.url+"/metrics")
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling metrics")
	}
	defer func() { _ = body.Close() }()
	return c.metricsParser.TextToMetricFamilies(body)
}

func (c *Client) get(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create http request to [%s]", url)
	}
	resp, err := c.c.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process http request to [%s]", url)
	}
	if resp.StatusCode != http.StatusOK {
		// Read and close the body so the transport can reuse the connection.
		// The read is capped so a large error body cannot hold the caller; Close handles the rest.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		_ = resp.Body.Close()
		return nil, errors.Errorf("failed to process http request to [%s], status code [%d], status [%s], body [%s]", url, resp.StatusCode, resp.Status, body)
	}
	return resp.Body, nil
}
