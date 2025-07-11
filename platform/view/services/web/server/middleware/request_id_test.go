/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"net/http"
	"net/http/httptest"

	middleware2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server/middleware/fakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WithRequestID", func() {
	var (
		requestID middleware2.Middleware
		handler   *fakes.HTTPHandler
		chain     http.Handler

		req  *http.Request
		resp *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		handler = &fakes.HTTPHandler{}
		requestID = middleware2.WithRequestID(
			middleware2.GenerateIDFunc(func() string { return "generated-id" }),
		)
		chain = requestID(handler)

		req = httptest.NewRequest("GET", "/", nil)
		resp = httptest.NewRecorder()
	})

	It("propagates the generated request ID in the request context", func() {
		chain.ServeHTTP(resp, req)
		_, r := handler.ServeHTTPArgsForCall(0)
		requestID := middleware2.RequestID(r.Context())
		Expect(requestID).To(Equal("generated-id"))
	})

	It("returns the generated request ID in a header", func() {
		chain.ServeHTTP(resp, req)
		Expect(resp.Result().Header.Get("X-Request-Id")).To(Equal("generated-id"))
	})

	Context("when a request ID is already present", func() {
		BeforeEach(func() {
			req.Header.Set("X-Request-Id", "received-id")
		})

		It("sets the received ID into the context", func() {
			chain.ServeHTTP(resp, req)
			_, r := handler.ServeHTTPArgsForCall(0)
			requestID := middleware2.RequestID(r.Context())
			Expect(requestID).To(Equal("received-id"))
		})

		It("sets the received ID into the request", func() {
			chain.ServeHTTP(resp, req)
			_, r := handler.ServeHTTPArgsForCall(0)
			Expect(r.Header.Get("X-Request-Id")).To(Equal("received-id"))
		})

		It("propagates the request ID to the response", func() {
			chain.ServeHTTP(resp, req)
			Expect(resp.Result().Header.Get("X-Request-Id")).To(Equal("received-id"))
		})
	})
})
