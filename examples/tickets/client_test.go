/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tickets_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	nwoclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	viewclient "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/client"
)

// restClient calls one node's REST API the way an existing system would: over
// HTTPS, presenting a client certificate. nwo's clients present the node's own
// TLS certificate, which is the one the API trusts.
type restClient struct {
	*viewclient.HTTPClient
}

func newRESTClient(ii *integration.Infrastructure, node string) *restClient {
	c, err := viewclient.NewHTTPClient(connection(ii, node))
	Expect(err).NotTo(HaveOccurred())
	return &restClient{HTTPClient: c}
}

// newCertlessClient trusts the node's server certificate but presents none of
// its own.
func newCertlessClient(ii *integration.Infrastructure, node string) *restClient {
	cc := connection(ii, node)
	cc.TLS.RequireClientCert = false
	c, err := viewclient.NewHTTPClient(cc)
	Expect(err).NotTo(HaveOccurred())
	return &restClient{HTTPClient: c}
}

func connection(ii *integration.Infrastructure, node string) *grpc.ConnectionConfig {
	for _, r := range ii.FscPlatform.Peers {
		if r.Name == node {
			cc, err := nwoclient.NewWebConnectionConfigFromFSC(ii.FscPlatform.NodeDir(r))
			Expect(err).NotTo(HaveOccurred())
			return cc
		}
	}
	Fail("no FSC node named " + node)
	return nil
}

// do sends a request with an optional JSON body and returns the status and the
// raw response body.
func (c *restClient) do(method, path string, body any) (int, []byte) {
	var in io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		in = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.URL+path, in)
	Expect(err).NotTo(HaveOccurred())
	resp, err := c.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return resp.StatusCode, out
}

// errorCode returns the "error" field of an error body.
func errorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	Expect(json.Unmarshal(body, &e)).To(Succeed(), string(body))
	return e.Error
}
