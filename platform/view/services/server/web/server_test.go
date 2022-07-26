/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hyperledger/fabric/core/operations/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc/tlsgen"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	fakes3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web/fakes"
	mocks2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web/mocks"
)

var (
	tlsCA tlsgen.CA
)

func init() {
	tlsCA, _ = tlsgen.NewCA()
}

func TestFabHTTP(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FabHTTP Suite")
}

func generateCertificates(tempDir string) {
	err := ioutil.WriteFile(filepath.Join(tempDir, "server-ca.pem"), tlsCA.CertBytes(), 0640)
	Expect(err).NotTo(HaveOccurred())
	serverKeyPair, err := tlsCA.NewServerCertKeyPair("127.0.0.1")
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-cert.pem"), serverKeyPair.Cert, 0640)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "server-key.pem"), serverKeyPair.Key, 0640)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(filepath.Join(tempDir, "client-ca.pem"), tlsCA.CertBytes(), 0640)
	Expect(err).NotTo(HaveOccurred())
	clientKeyPair, err := tlsCA.NewClientCertKeyPair()
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-cert.pem"), clientKeyPair.Cert, 0640)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile(filepath.Join(tempDir, "client-key.pem"), clientKeyPair.Key, 0640)
	Expect(err).NotTo(HaveOccurred())
}

func newHTTPClient(tlsDir string, withClientCert bool) *http.Client {
	clientCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(filepath.Join(tlsDir, "server-ca.pem"))
	Expect(err).NotTo(HaveOccurred())
	clientCertPool.AppendCertsFromPEM(caCert)

	tlsClientConfig := &tls.Config{
		RootCAs: clientCertPool,
	}
	if withClientCert {
		clientCert, err := tls.LoadX509KeyPair(
			filepath.Join(tlsDir, "client-cert.pem"),
			filepath.Join(tlsDir, "client-key.pem"),
		)
		Expect(err).NotTo(HaveOccurred())
		tlsClientConfig.Certificates = []tls.Certificate{clientCert}
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig,
		},
	}
}

var _ = Describe("Server", func() {
	const someURL = "/some-URL"

	var (
		fakeLogger *fakes3.Logger
		tempDir    string

		client  *http.Client
		options web2.Options
		server  *web2.Server
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "http-test")
		Expect(err).NotTo(HaveOccurred())

		generateCertificates(tempDir)
		client = newHTTPClient(tempDir, true)

		fakeLogger = &fakes3.Logger{}
		options = web2.Options{
			Logger:        fakeLogger,
			ListenAddress: "127.0.0.1:0",
			TLS: web2.TLS{
				Enabled:           true,
				CertFile:          filepath.Join(tempDir, "server-cert.pem"),
				KeyFile:           filepath.Join(tempDir, "server-key.pem"),
				ClientCACertFiles: []string{filepath.Join(tempDir, "client-ca.pem")},
			},
		}

		server = web2.NewServer(options)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		if server != nil {
			server.Stop()
		}
	})

	When("the HttpHandler is mounted on the server", func() {
		It("succeeds in servicing", func() {
			handler := web2.NewHttpHandler(fakeLogger)
			server.RegisterHandler("/", handler, true)
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			rh := &mocks2.FakeRequestHandler{}

			rh.HandleRequestStub = func(ctx *web2.ReqContext) (interface{}, int) {
				m := make(map[string]interface{})
				m["status"] = "OK"
				return m, 200
			}

			rh.ParsePayloadStub = func(payload []byte) (interface{}, error) {
				return string(payload), nil
			}

			handler.RegisterURI("/service", "GET", rh)

			url := fmt.Sprintf("https://%s%s", server.Addr(), "/v1/service")
			resp, err := client.Get(url)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
			buff, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Trim(string(buff), "\n")).To(Equal(`{"status":"OK"}`))
			resp.Body.Close()

		})
	})

	When("a client connects without mutual TLS", func() {
		BeforeEach(func() {
			client = newHTTPClient(tempDir, false)
		})
		It("is rejected", func() {
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			url := fmt.Sprintf("https://%s%s", server.Addr(), someURL)
			resp, err := client.Get(url)
			Expect(err.Error()).To(ContainSubstring("remote error: tls: bad certificate"))
			Expect(resp).To(BeNil())
		})
	})

	It("hosts a secure endpoint for additional APIs when added", func() {
		server.RegisterHandler(someURL, &fakes.Handler{Code: http.StatusOK, Text: "secure"}, true)
		err := server.Start()
		Expect(err).NotTo(HaveOccurred())

		addApiURL := fmt.Sprintf("https://%s%s", server.Addr(), someURL)
		resp, err := client.Get(addApiURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/plain; charset=utf-8"))
		buff, err := ioutil.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(buff)).To(Equal("secure"))
		resp.Body.Close()
	})

	Context("when TLS is disabled", func() {
		BeforeEach(func() {
			options.TLS.Enabled = false
			server = web2.NewServer(options)
		})

		It("does not host an insecure endpoint for additional APIs by default", func() {
			err := server.Start()
			Expect(err).NotTo(HaveOccurred())

			addApiURL := fmt.Sprintf("http://%s%s", server.Addr(), someURL)
			resp, err := client.Get(addApiURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
			resp.Body.Close()
		})
	})

	Context("when listen fails", func() {
		var listener net.Listener

		BeforeEach(func() {
			var err error
			listener, err = net.Listen("tcp", "127.0.0.1:0")
			Expect(err).NotTo(HaveOccurred())

			options.ListenAddress = listener.Addr().String()
			server = web2.NewServer(options)
		})

		AfterEach(func() {
			listener.Close()
		})

		It("returns an error", func() {
			err := server.Start()
			Expect(err).To(MatchError(ContainSubstring("bind: address already in use")))
		})
	})

	Context("when a bad TLS configuration is provided", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "cert-file-does-not-exist"
			server = web2.NewServer(options)
		})

		It("returns an error", func() {
			err := server.Start()
			Expect(err).To(MatchError("open cert-file-does-not-exist: no such file or directory"))
		})
	})

	It("supports ifrit", func() {
		process := ifrit.Invoke(server)
		Eventually(process.Ready()).Should(BeClosed())

		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait()).Should(Receive(BeNil()))
	})

	Context("when start fails and ifrit is used", func() {
		BeforeEach(func() {
			options.TLS.CertFile = "non-existent-file"
			server = web2.NewServer(options)
		})

		It("does not close the ready chan", func() {
			process := ifrit.Invoke(server)
			Consistently(process.Ready()).ShouldNot(BeClosed())
			Eventually(process.Wait()).Should(Receive(MatchError("open non-existent-file: no such file or directory")))
		})
	})
})
