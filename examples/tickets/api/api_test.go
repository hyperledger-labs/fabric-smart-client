/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestStatusOf(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{views.ErrInvalidInput, http.StatusBadRequest, "bad_request"},
		{views.ErrUnknownEvent, http.StatusNotFound, "not_found"},
		{views.ErrSeatSold, http.StatusConflict, "already_sold"},
		{views.ErrLostRace, http.StatusConflict, "lost_race"},
		{views.ErrConflict, http.StatusConflict, "conflict"},
		{views.ErrRefused, http.StatusUnprocessableEntity, "refused"},
		{views.ErrUnreachable, http.StatusServiceUnavailable, "unavailable"},
		{views.ErrPending, http.StatusGatewayTimeout, "pending"},
		{errors.New("boom"), http.StatusInternalServerError, "internal"},
	} {
		status, code := statusOf(errors.WithMessage(c.err, "context"))
		require.Equal(t, c.status, status, c.code)
		require.Equal(t, c.code, code)
	}
}

func selfSigned(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert
}

func TestTrusted(t *testing.T) {
	t.Parallel()
	own, other := selfSigned(t, "own"), selfSigned(t, "other")
	req := func(certs ...*x509.Certificate) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/events", nil)
		r.TLS = &tls.ConnectionState{PeerCertificates: certs}
		return r
	}
	require.True(t, trusted(req(own), own.Raw))
	require.False(t, trusted(req(other), own.Raw))
	require.False(t, trusted(req(), own.Raw))
	plain := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	require.False(t, trusted(plain, own.Raw))
}

func TestBuyRejectsBadSeat(t *testing.T) {
	t.Parallel()
	// No view manager: a bad seat must be rejected before any view runs.
	h := &handlers{}
	for seat, want := range map[string]int{"x": http.StatusBadRequest, "0": http.StatusNotFound, "201": http.StatusNotFound} {
		r := httptest.NewRequest(http.MethodPost, "/api/events/e1/seats/"+seat, nil)
		r.SetPathValue("id", "e1")
		r.SetPathValue("n", seat)
		w := httptest.NewRecorder()
		h.buy(w, r)
		require.Equal(t, want, w.Code, seat)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/events/e1/seats/3?hold=1h", nil)
	r.SetPathValue("id", "e1")
	r.SetPathValue("n", "3")
	w := httptest.NewRecorder()
	h.buy(w, r)
	require.Equal(t, http.StatusBadRequest, w.Code, "hold above the limit")
}
