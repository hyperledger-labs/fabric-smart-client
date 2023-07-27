/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("view-sdk.web.client")

// Config models the configuration for the web client
type Config struct {
	// Host to connect to
	Host string
	// CACert is the Certificate Authority Cert Path
	CACert string
	// TLSCert is the TLS client certificate path
	TLSCert string
	// TLSKey is the TLS client key path
	TLSKey string
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
	return c.CACert != ""
}

// Client models a client for an FSC node
type Client struct {
	c         *http.Client
	url       string
	wsUrl     string
	tlsConfig *tls.Config
}

// NewClient returns a new web client
func NewClient(config *Config) (*Client, error) {
	var tlsClientConfig *tls.Config

	if len(config.CACert) != 0 {
		rootCAs := x509.NewCertPool()
		caCert, err := os.ReadFile(config.CACert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open ca cert")
		}
		rootCAs.AppendCertsFromPEM(caCert)
		tlsClientConfig = &tls.Config{
			RootCAs: rootCAs,
		}

		if len(config.TLSCert) != 0 && len(config.TLSKey) != 0 {
			clientCert, err := tls.LoadX509KeyPair(
				config.TLSCert,
				config.TLSKey,
			)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to load x509 key pair")
			}
			tlsClientConfig.Certificates = []tls.Certificate{clientCert}
		}
	}

	return &Client{
		c: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
		},
		url:       config.WebURL(),
		wsUrl:     config.WsURL(),
		tlsConfig: tlsClientConfig,
	}, nil
}

func (c *Client) StreamCallView(fid string) (*WsServiceStream, error) {
	urlSuffix := fmt.Sprintf("/v1/Views/Stream/%s", fid)
	return NewWsStream(c.wsUrl+urlSuffix, c.tlsConfig)
}

// CallView takes in input a view factory identifier, fid, and an input, in, and invokes the
// factory f bound to fid on input in. The view returned by the factory is invoked on
// a freshly created context. This call is blocking until the result is produced or
// an error is returned.
func (c *Client) CallView(fid string, in []byte) (interface{}, error) {
	url := fmt.Sprintf("%s/v1/Views/%s", c.url, fid)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(in))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create http request to [%s], input length [%d]", url, len(in))
	}
	logger.Debugf("call view [%s] using http request to [%s], input length [%d]", fid, url, len(in))

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
	defer resp.Body.Close()
	buff, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read response from http request to [%s], input length [%d]", url, len(in))
	}

	response := &protos2.CommandResponse_CallViewResponse{}
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

func (c *Client) Track(cid string) string {
	panic("implement me")
}

func (c *Client) IsTxFinal(txid string, opts ...api.ServiceOption) error {
	panic("implement me")
}

func (c *Client) ServerVersion() (string, error) {
	url := fmt.Sprintf("%s/version", c.url)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create http request to [%s]", url)
	}
	logger.Debugf("version using http request to [%s]", url)

	resp, err := c.c.Do(req)
	if err != nil {
		return "", errors.Wrapf(err, "failed to process http request to [%s]", url)
	}
	if resp == nil {
		return "", errors.Errorf("failed to process http request to [%s], no response", url)
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed to process http request to [%s], status code [%d], status [%s]", url, resp.StatusCode, resp.Status)
	}
	defer resp.Body.Close()
	buff, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read response from http request to [%s]", url)
	}
	return string(buff), nil
}
