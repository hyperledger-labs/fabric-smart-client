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
	"io/ioutil"
	"net/http"

	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

// Config models the configuration for the web client
type Config struct {
	// URL to connect to
	URL string
	// CACert is the Certificate Authority Cert Path
	CACert string
	// TLSCert is the TLS client certificate path
	TLSCert string
	// TLSKey is the TLS client key path
	TLSKey string
}

// Client models a client for an FSC node
type Client struct {
	c   *http.Client
	url string
}

// NewClient returns a new web client
func NewClient(config *Config) (*Client, error) {
	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(config.CACert)
	if err != nil {
		return nil, err
	}
	clientCertPool.AppendCertsFromPEM(caCert)

	tlsClientConfig := &tls.Config{
		RootCAs: clientCertPool,
	}
	clientCert, err := tls.LoadX509KeyPair(
		config.TLSCert,
		config.TLSKey,
	)
	if err != nil {
		return nil, err
	}
	tlsClientConfig.Certificates = []tls.Certificate{clientCert}

	return &Client{
		c: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
		},
		url: config.URL,
	}, nil
}

// CallView takes in input a view factory identifier, fid, and an input, in, and invokes the
// factory f bound to fid on input in. The view returned by the factory is invoked on
// a freshly created context. This call is blocking until the result is produced or
// an error is returned.
func (c *Client) CallView(fid string, in []byte) (interface{}, error) {
	url := fmt.Sprintf("%s/v1/Views/%s", c.url, fid)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer([]byte("hi")))
	if err != nil {
		return nil, err
	}

	resp, err := c.c.Do(req)
	if err != nil {
		return nil, err
	}
	buff, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	response := &protos2.CommandResponse_CallViewResponse{}
	err = json.Unmarshal(buff, response)
	if err != nil {
		return nil, err
	}
	return response.CallViewResponse.Result, nil
}

func (c *Client) Initiate(fid string, in []byte) (string, error) {
	panic("implement me")
}

func (c *Client) Track(cid string) string {
	panic("implement me")
}

func (c *Client) IsTxFinal(txid string) error {
	panic("implement me")
}
