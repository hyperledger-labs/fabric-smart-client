/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/client"
)

var _ = Describe("Config", func() {
	var config client.Config

	BeforeEach(func() {
		config = client.Config{
			ConnectionConfig: &grpc.ConnectionConfig{
				Address: "127.0.0.1:0",
				TLS: grpc.SecureOptions{
					UseTLS:        true,
					ServerRootCAs: [][]byte{[]byte("cert")},
				},
			},
		}
	})

	Describe("ValidateConfig", func() {
		It("returns no error for validate config", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when there is no fsc address", func() {
		BeforeEach(func() {
			config.ConnectionConfig.Address = ""
		})

		It("returns missing fsc address error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing fsc peer address"))
		})
	})

	// Validation now checks the resolved pool rather than the two configuration keys it used
	// to be assembled from: a TLS-enabled connection with no anchor could never verify the
	// server, however the anchors were configured.
	Context("when TLS is enabled with no root certificates", func() {
		BeforeEach(func() {
			config.ConnectionConfig.TLS.ServerRootCAs = nil
		})

		It("returns a missing root certificates error", func() {
			err := client.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing fsc peer TLS root certificates"))
		})
	})
})
