/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fsc/pingpong"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
)

var _ = Describe("EndToEnd", func() {

	Describe("Node-based Ping pong", func() {

		It("successful pingpong based on REST API", func() {
			// Init and Start fsc nodes
			initiator := node.NewFromConfPath("./testdata/fsc/fscnodes/fsc.initiator")
			Expect(initiator).NotTo(BeNil())

			responder := node.NewFromConfPath("./testdata/fsc/fscnodes/fsc.responder")
			Expect(responder).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			responder.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

			time.Sleep(3 * time.Second)

			client := newHTTPClient("./testdata/fsc/fscnodes/fsc.initiator")

			url := "https://127.0.0.1:19999/v1/mychannel/Views/init"
			req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer([]byte("hi")))
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			buff, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			response := &protos.CommandResponse_CallViewResponse{}
			err = json.Unmarshal(buff, response)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(response.CallViewResponse.Result)).To(Equal("\"OK\""))

			initiator.Stop()
			responder.Stop()
		})

		It("successful pingpong", func() {
			// Init and Start fsc nodes
			initiator := node.NewFromConfPath("./testdata/fsc/fscnodes/fsc.initiator")
			Expect(initiator).NotTo(BeNil())

			responder := node.NewFromConfPath("./testdata/fsc/fscnodes/fsc.responder")
			Expect(responder).NotTo(BeNil())

			err := initiator.Start()
			Expect(err).NotTo(HaveOccurred())
			err = responder.Start()
			Expect(err).NotTo(HaveOccurred())

			// Register views and view factories
			err = initiator.RegisterFactory("init", &pingpong.InitiatorViewFactory{})
			Expect(err).NotTo(HaveOccurred())
			responder.RegisterResponder(&pingpong.Responder{}, &pingpong.Initiator{})

			time.Sleep(3 * time.Second)
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))

			initiator.Stop()
			responder.Stop()
		})

	})

	Describe("Network-based Ping pong", func() {
		var (
			ii *integration.Infrastructure
		)

		AfterEach(func() {
			// Stop the ii
			ii.Stop()
		})

		It("generate artifacts & successful pingpong", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPortWithGeneration(), pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("generate artifacts & successful pingpong with Admin", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Generate(StartPortWithAdmin(), pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get an admin client for the fsc node labelled initiator
			initiatorAdmin := ii.Admin("initiator")
			// Initiate a view and check the output
			res, err := initiatorAdmin.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

		It("load artifact & successful pingpong", func() {
			var err error
			// Create the integration ii
			ii, err = integration.Load("./testdata", pingpong.Topology()...)
			Expect(err).NotTo(HaveOccurred())
			// Start the integration ii
			ii.Start()
			time.Sleep(3 * time.Second)
			// Get a client for the fsc node labelled initiator
			initiator := ii.Client("initiator")
			// Initiate a view and check the output
			res, err := initiator.CallView("init", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(common.JSONUnmarshalString(res)).To(BeEquivalentTo("OK"))
		})

	})

})

func newHTTPClient(confDir string) *http.Client {
	configProvider, err := config2.NewProvider(confDir)
	Expect(err).NotTo(HaveOccurred())

	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(configProvider.TranslatePath(configProvider.GetStringSlice("fsc.tls.clientRootCAs.files")[0]))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	tlsClientConfig := &tls.Config{
		RootCAs: clientCertPool,
	}
	clientCert, err := tls.LoadX509KeyPair(
		configProvider.GetPath("fsc.tls.cert.file"),
		configProvider.GetPath("fsc.tls.key.file"))
	Expect(err).NotTo(HaveOccurred())
	tlsClientConfig.Certificates = []tls.Certificate{clientCert}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
	}
}
