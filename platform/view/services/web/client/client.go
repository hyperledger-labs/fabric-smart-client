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

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"gopkg.in/yaml.v2"
)

var logger = logging.MustGetLogger()

// Config models the configuration for the web client
type Config struct {
	Address   string
	TLSConfig TLSClientConfig
}

// TLSClientConfig defines configuration of a Client
type TLSClientConfig struct {
	Enabled            bool   `yaml:"enabled"`
	RootCACertPath     string `yaml:"rootCACertPath,omitempty"`
	ClientAuthRequired bool   `yaml:"clientAuthRequired,omitempty"`
	ClientCertPath     string `yaml:"clientCertPath,omitempty"`
	ClientKeyPath      string `yaml:"clientKeyPath,omitempty"`
}

// ConfigFromFile loads the given file and converts it to a Config
func ConfigFromFile(file string) (*Config, error) {
	configData, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var config Config

	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, errors.Errorf("error unmarshalling YAML file %s: %s", file, err)
	}

	return &config, nil
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
	return fmt.Sprintf("%s://%s", protocol, c.Address)
}
func (c *Config) isTlsEnabled() bool {
	return c.TLSConfig.Enabled
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
	tlsConfig, err := createTLSConfig(config.TLSConfig)
	if err != nil {
		return nil, err
	}

	if tlsConfig == nil {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &Client{
		c: &http.Client{
			Transport: otelhttp.NewTransport(&http.Transport{
				TLSClientConfig: tlsConfig,
			}),
		},
		url:           config.WebURL(),
		wsUrl:         config.WsURL(),
		tlsConfig:     tlsConfig,
		metricsParser: expfmt.NewTextParser(model.LegacyValidation),
	}, nil
}

// createTLSConfig returns tls.Config based on the passed TLSClientConfig.
// It returns nil and an error if there is no valid TLS configuration provided.
// If TLS is not enabled, we return nil.
func createTLSConfig(config TLSClientConfig) (*tls.Config, error) {
	var cfg tls.Config

	if !config.Enabled {
		return nil, nil
	}

	if len(config.RootCACertPath) < 1 {
		return nil, errors.New("RootCACertPath is not set")
	}

	caCert, err := os.ReadFile(config.RootCACertPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open ca cert")
	}

	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(caCert)
	cfg.RootCAs = rootCAs

	if !config.ClientAuthRequired {
		return &cfg, nil
	}

	// mTLS
	clientCert, err := tls.LoadX509KeyPair(
		config.ClientCertPath,
		config.ClientKeyPath,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load x509 key pair")
	}
	cfg.Certificates = []tls.Certificate{clientCert}

	return &cfg, nil
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
