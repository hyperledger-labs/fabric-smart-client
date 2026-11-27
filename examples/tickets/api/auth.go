/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"net/http"
)

// trusted reports whether the request's client certificate is cert (DER).
//
// The web listener's mTLS already checked that the certificate comes from a
// CA the node trusts. In this demo's network every node shares one TLS CA, so
// that alone would let reseller-us call reseller-eu's API. The API therefore
// also accepts only one certificate: the one its own system holds, which here
// is the node's own TLS certificate.
func trusted(r *http.Request, cert []byte) bool {
	return r.TLS != nil && len(r.TLS.PeerCertificates) > 0 && bytes.Equal(r.TLS.PeerCertificates[0].Raw, cert)
}

func (h *handlers) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !trusted(r, h.cert) {
			writeError(w, http.StatusForbidden, "forbidden", "client certificate is not trusted by this node")
			return
		}
		next(w, r)
	}
}
