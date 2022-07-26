/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var _ = Describe("Config", func() {
	var (
		config view.Config
	)

	BeforeEach(func() {
		config = view.Config{
			ConnectionConfig: &grpc.ConnectionConfig{
				Address:         "127.0.0.1:0",
				TLSEnabled:      true,
				TLSRootCertFile: "root-ca",
			},
		}
	})

	Describe("ValidateConfig", func() {
		It("returns no error for validate config", func() {
			err := view.ValidateClientConfig(config)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when there is no fsc address", func() {
		BeforeEach(func() {
			config.ConnectionConfig.Address = ""
		})

		It("returns missing fsc address error", func() {
			err := view.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing fsc peer address"))
		})
	})

	Context("when there is no fsc TLSRootCertFile", func() {
		BeforeEach(func() {
			config.ConnectionConfig.TLSRootCertFile = ""
		})

		It("returns fsc TLSRootCertFile error", func() {
			err := view.ValidateClientConfig(config)
			Expect(err).To(MatchError("missing fsc peer TLSRootCertFile"))
		})
	})
})
