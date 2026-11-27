/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
)

func render(t *testing.T, p *page) string {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, pages.ExecuteTemplate(&b, "page.html", p))
	return b.String()
}

func TestResellerPageMarksSeats(t *testing.T) {
	t.Parallel()
	e := &Event{
		ID: "e1", Name: "Neon Nights", Seats: 3, Price: 50,
		Seating: []Seat{
			{N: 1, Sold: true, Reseller: "reseller-eu"},
			{N: 2, Sold: true, Reseller: "reseller-us"},
			{N: 3},
		},
	}
	out := render(t, &page{Node: "reseller-eu", Events: []*Event{e}, Selected: e})

	require.Regexp(t, `<button[^>]*class="seat primary"[^>]*disabled[^>]*>1</button>`, out)
	require.Regexp(t, `<button[^>]*class="seat surface-variant"[^>]*disabled[^>]*>2</button>`, out)
	require.Contains(t, out, `formaction="/ui/events/e1/seats/3"`)
	require.NotContains(t, out, `formaction="/ui/events/e1/seats/1"`)
}

func TestPageShowsErrorBanner(t *testing.T) {
	t.Parallel()
	out := render(t, &page{Node: "reseller-eu", ErrCode: "lost_race", Err: "seat went to reseller-us"})
	require.Regexp(t, `(?s)class="[^"]*error-container[^"]*".*Lost the race`, out)
}

func TestPromoterPageEscapesInput(t *testing.T) {
	t.Parallel()
	out := render(t, &page{Node: views.PromoterNode, Promoter: true, OK: "<script>x</script>"})
	require.Contains(t, out, "New event")
	require.NotContains(t, out, "<script>x</script>")
}

func TestPromoterFormRejectsBadPrice(t *testing.T) {
	t.Parallel()
	h := &handlers{name: views.PromoterNode}
	form := url.Values{"name": {"A"}, "artist": {"B"}, "venue": {"C"}, "date": {"2027-03-02"}, "seats": {"10"}, "price": {"abc"}}
	r := httptest.NewRequest(http.MethodPost, "/ui/events", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.uiCreate(w, r)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "error-container")
	require.Contains(t, w.Body.String(), "price")
}

func TestUIRejectsCrossOriginPosts(t *testing.T) {
	t.Parallel()
	ws := &fakeWebServer{handlers: map[string]http.Handler{}}
	mountUI(ws, &handlers{name: "reseller-eu"})
	r := httptest.NewRequest(http.MethodPost, "/ui/events/e1/seats/3", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	ws.handlers["POST /ui/events/{id}/seats/{n}"].ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
}

type fakeWebServer struct{ handlers map[string]http.Handler }

func (f *fakeWebServer) RegisterHandler(pattern string, h http.Handler, _ bool) {
	f.handlers[pattern] = h
}
