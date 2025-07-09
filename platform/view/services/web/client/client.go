/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"github.com/pkg/errors"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var logger = logging.MustGetLogger()

// Config models the configuration for the web client
type Config struct {
	// Host to connect to
	Host string
	// CACertRaw is the certificate authority's certificates
	CACertRaw []byte
	// CACertPath is the Certificate Authority Cert Path
	CACertPath string
	// TLSCertPath is the TLS client certificate path
	TLSCertPath string
	// TLSKeyPath is the TLS client key path
	TLSKeyPath string
}

func (c *Config) WsURL() string {
	return c.url("ws")
}

func (c *Config) WebURL() string {
	return c.url("http")
}

func (c *Config) url(protocol string) string {
	if c.isTlsEnabled() {
		protocol = protocol + "s"
	}
	return fmt.Sprintf("%s://%s", protocol, c.Host)
}
func (c *Config) isTlsEnabled() bool {
	return c.CACertPath != ""
}

// Client models a client for an FSC node
type Client struct {
	c             *http.Client
	url           string
	wsUrl         string
	tlsConfig     *tls.Config
	metricsParser expfmt.TextParser
}

// NewClient returns a new web client
func NewClient(config *Config) (*Client, error) {
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

		if len(config.TLSCertPath) != 0 && len(config.TLSKeyPath) != 0 {
			clientCert, err := tls.LoadX509KeyPair(
				config.TLSCertPath,
				config.TLSKeyPath,
			)
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
		url:       config.WebURL(),
		wsUrl:     config.WsURL(),
		tlsConfig: tlsClientConfig,
	}, nil
}

func (c *Client) Metrics() (map[string]*dto.MetricFamily, error) {
	body, err := c.req(context.Background(), http.MethodGet, fmt.Sprintf("%s/metrics", c.url), nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling metrics")
	}
	return c.metricsParser.TextToMetricFamilies(body)
}

func (c *Client) StreamCallView(fid string, in []byte) (*WSStream, error) {
	urlSuffix := fmt.Sprintf("/v1/Views/Stream/%s", fid)
	stream, err := NewWSStream(c.wsUrl+urlSuffix, c.tlsConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to init web socket stream")
	}
	// push input
	if err := stream.SendInput(in); err != nil {
		return nil, errors.WithMessage(err, "failed to send input")
	}
	return stream, nil
}

func (c *Client) req(ctx context.Context, method string, url string, in []byte) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(in))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create http request to [%s], input length [%d]", url, len(in))
	}
	logger.Debugf("send http request to [%s], input length [%d]", url, len(in))

	resp, err := c.c.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to process http request to [%s], input length [%d]", url, len(in))
	}
	if resp == nil {
		return nil, errors.Errorf("failed to process http request to [%s], input length [%d], no response", url, len(in))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to process http request to [%s], input length [%d], status code [%d], status [%s]", url, len(in), resp.StatusCode, resp.Status)
	}
	return resp.Body, nil
}

// CallView takes in input a view factory identifier, fid, and an input, in, and invokes the
// factory f bound to fid on input in. The view returned by the factory is invoked on
// a freshly created context. This call is blocking until the result is produced or
// an error is returned.
func (c *Client) CallView(fid string, in []byte) (interface{}, error) {
	return c.CallViewWithContext(context.Background(), fid, in)
}

func (c *Client) CallViewWithContext(ctx context.Context, fid string, in []byte) (interface{}, error) {
	url := fmt.Sprintf("%s/v1/Views/%s", c.url, fid)
	body, err := c.req(ctx, http.MethodPut, url, in)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call [%s]", fid)
	}
	defer utils.IgnoreErrorFunc(body.Close)
	buff, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read response from http request to [%s], input length [%d]", url, len(in))
	}

	response := &protos.CommandResponse_CallViewResponse{}
	err = json.Unmarshal(buff, response)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal response from [%s], response [%s]", url, string(buff))
	}
	if response.CallViewResponse == nil {
		return nil, errors.Errorf("invalid response from [%s], response [%s]", url, string(buff))
	}
	return response.CallViewResponse.Result, nil
}

func (c *Client) Initiate(fid string, in []byte) (string, error) {
	panic("implement me")
}
