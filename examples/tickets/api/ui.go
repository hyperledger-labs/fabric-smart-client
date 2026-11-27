/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// The web UI: server-rendered pages that call the same views as the API.
// Beer CSS (Material Design 3) is vendored under static/, so the pages work
// offline. The UI has no login: anyone who reaches the port can use it.

//go:embed templates static
var files embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"soldTotal": soldTotal,
	"seatsLeft": func(e *Event) int { return e.Seats - soldTotal(e.Sold) },
}).ParseFS(files, "templates/*.html"))

func soldTotal(sold map[string]int) int {
	total := 0
	for _, n := range sold {
		total += n
	}
	return total
}

// page is what templates/page.html renders.
type page struct {
	Node     string
	Promoter bool
	Events   []*Event
	// Selected is the event a reseller looks at, with its seating; Edit is the
	// event the promoter's form edits.
	Selected *Event
	Edit     *states.Event
	OK       string
	// Err and ErrCode describe a failed action; ErrCode is the API's error code.
	Err, ErrCode string
	// Curl and Response show the REST call equivalent to the last buy.
	Curl, Response string
}

// Banner returns the error banner's headline.
func (p *page) Banner() string {
	switch p.ErrCode {
	case "already_sold":
		return "Already sold: the seat was taken before we asked the promoter."
	case "lost_race":
		return "Lost the race: another reseller's sale committed first; the ledger rejected ours."
	case "conflict":
		return "The event changed while we waited; try again."
	case "pending":
		return "The sale was submitted, but the ledger has not confirmed it yet. Check the seat before selling it again."
	}
	return "Something went wrong."
}

func mountUI(ws WebServer, h *handlers) {
	ws.RegisterHandler("GET /{$}", http.HandlerFunc(h.uiPage), false)
	ws.RegisterHandler("GET /static/", http.FileServerFS(files), false)
	// The forms take no login, but they accept posts only from the node's own
	// page, so another web site the user visits cannot submit them.
	post := func(pattern string, f http.HandlerFunc) {
		ws.RegisterHandler(pattern, http.NewCrossOriginProtection().Handler(f), false)
	}
	if h.name == views.PromoterNode {
		post("POST /ui/events", h.uiCreate)
		post("POST /ui/events/{id}", h.uiUpdate)
	} else {
		post("POST /ui/events/{id}/seats/{n}", h.uiBuy)
	}
}

// newPage loads the events and, from ?event= or ?edit=, the selected one.
func (h *handlers) newPage(q url.Values) (*page, error) {
	p := &page{Node: h.name, Promoter: h.name == views.PromoterNode, OK: q.Get("ok")}
	var err error
	if p.Events, err = h.events(); err != nil {
		return p, err
	}
	if id := q.Get("event"); id != "" {
		if p.Selected, err = h.event(id, true); err != nil {
			return p, err
		}
	}
	if id := q.Get("edit"); id != "" {
		for _, e := range p.Events {
			if e.ID == id {
				p.Edit = &e.Event
			}
		}
	}
	return p, nil
}

func (h *handlers) uiPage(w http.ResponseWriter, r *http.Request) {
	p, err := h.newPage(r.URL.Query())
	show(w, p, err)
}

// show renders p; with err set, it shows the error banner and answers with
// the status the API would have used.
func show(w http.ResponseWriter, p *page, err error) {
	status := http.StatusOK
	if err != nil {
		status, p.ErrCode = statusOf(err)
		p.Err = err.Error()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = pages.ExecuteTemplate(w, "page.html", p)
}

// fail re-renders the page with an error, keeping what loads.
func (h *handlers) fail(w http.ResponseWriter, q url.Values, err error) {
	p := &page{Node: h.name, Promoter: h.name == views.PromoterNode}
	if h.sp != nil {
		if loaded, lerr := h.newPage(q); lerr == nil {
			p = loaded
		}
	}
	show(w, p, err)
}

// eventForm reads the promoter's form.
func eventForm(r *http.Request) (states.Event, error) {
	seats, err := strconv.Atoi(r.PostFormValue("seats"))
	if err != nil {
		return states.Event{}, errors.Wrap(views.ErrInvalidInput, "seats must be a number")
	}
	price, err := strconv.ParseUint(r.PostFormValue("price"), 10, 64)
	if err != nil {
		return states.Event{}, errors.Wrap(views.ErrInvalidInput, "price must be a whole number")
	}
	return states.Event{
		Name: r.PostFormValue("name"), Artist: r.PostFormValue("artist"), Venue: r.PostFormValue("venue"),
		Date: r.PostFormValue("date"), Seats: seats, Price: price,
	}, nil
}

func (h *handlers) uiCreate(w http.ResponseWriter, r *http.Request) {
	e, err := eventForm(r)
	if err != nil {
		h.fail(w, nil, err)
		return
	}
	out, err := h.mgr.InitiateView(r.Context(), &views.CreateEventView{Event: e})
	if err != nil {
		h.fail(w, nil, err)
		return
	}
	created, _ := out.(*states.Event)
	// Redirect after the post, so reloading the page does not create it again.
	http.Redirect(w, r, "/?ok="+url.QueryEscape("Created "+created.Name), http.StatusSeeOther)
}

func (h *handlers) uiUpdate(w http.ResponseWriter, r *http.Request) {
	e, err := eventForm(r)
	if err != nil {
		h.fail(w, url.Values{"edit": {r.PathValue("id")}}, err)
		return
	}
	e.ID = r.PathValue("id")
	if _, err := h.mgr.InitiateView(r.Context(), &views.UpdateEventView{Event: e}); err != nil {
		h.fail(w, url.Values{"edit": {e.ID}}, err)
		return
	}
	http.Redirect(w, r, "/?ok="+url.QueryEscape("Updated "+e.Name), http.StatusSeeOther)
}

// uiBuy renders its result directly instead of redirecting, so the page can
// show the REST call and its response. Reloading it buys the same seat again,
// which only fails with "already sold".
func (h *handlers) uiBuy(w http.ResponseWriter, r *http.Request) {
	id, n, hold := r.PathValue("id"), r.PathValue("n"), ""
	if r.PostFormValue("hold") != "" {
		hold = "5s"
	}
	in, err := parseBuy(id, n, hold)
	var out any
	if err == nil {
		out, err = h.mgr.InitiateView(r.Context(), &views.BuyView{Buy: in})
	}

	// Load the page after the buy, so the seat grid shows the result.
	p, lerr := h.newPage(url.Values{"event": {id}})
	p.Curl = "curl --cacert ca.crt --cert server.crt --key server.key -X POST 'https://" + r.Host +
		"/api/events/" + id + "/seats/" + n + "?hold=" + hold + "'"
	if err != nil {
		status, code := statusOf(err)
		raw, _ := json.MarshalIndent(map[string]string{"error": code, "message": err.Error()}, "", "  ")
		p.Response = strconv.Itoa(status) + " " + string(raw)
		show(w, p, err)
		return
	}
	seat, _ := out.(*states.Seat)
	raw, _ := json.MarshalIndent(seat, "", "  ")
	p.Response = "201 " + string(raw)
	p.OK = "Sold seat " + n + ". Ticket reference: " + seat.Ticket
	show(w, p, lerr)
}
