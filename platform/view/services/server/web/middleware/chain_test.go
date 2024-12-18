/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package middleware_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Chain", func() {
	var (
		one, two, three middleware.Middleware
		chain           middleware.Chain

		hello http.Handler

		req  *http.Request
		resp *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		one = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(w.Write([]byte("1:before,"))).To(Succeed())
				next.ServeHTTP(w, r)
				Expect(w.Write([]byte("1:after"))).To(Succeed())
			})
		}
		two = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(w.Write([]byte("2:before,"))).To(Succeed())
				next.ServeHTTP(w, r)
				Expect(w.Write([]byte("2:after,"))).To(Succeed())
			})
		}
		three = func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(w.Write([]byte("3:before,"))).To(Succeed())
				next.ServeHTTP(w, r)
				Expect(w.Write([]byte("3:after,"))).To(Succeed())
			})
		}
		chain = middleware.NewChain(one, two, three)

		hello = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			Expect(w.Write([]byte("Hello!,"))).To(Succeed())
		})

		req = httptest.NewRequest("GET", "/", nil)
		resp = httptest.NewRecorder()
	})

	It("calls middleware in the specified order", func() {
		chain.Handler(hello).ServeHTTP(resp, req)
		Expect(resp.Body.String()).To(Equal("1:before,2:before,3:before,Hello!,3:after,2:after,1:after"))
	})

	Context("when the chain is empty", func() {
		BeforeEach(func() {
			chain = middleware.NewChain()
		})

		It("calls the handler", func() {
			chain.Handler(hello).ServeHTTP(resp, req)
			Expect(resp.Body.String()).To(Equal("Hello!,"))
		})
	})

	Context("when the handler is nil", func() {
		It("uses the DefaultServerMux", func() {
			chain.Handler(nil).ServeHTTP(resp, req)
			Expect(resp.Body.String()).To(ContainSubstring("404 page not found"))
		})
	})
})
