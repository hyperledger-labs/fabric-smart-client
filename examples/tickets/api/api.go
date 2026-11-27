/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package api is the ticket demo's REST API and web UI. Both are plain
// net/http handlers mounted on the FSC node's web listener; they call the
// views in the same process.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/views"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
)

// WebServer is the node's web listener (fsc.web). With secure set, a request
// needs a client certificate from a CA the listener trusts.
type WebServer interface {
	RegisterHandler(pattern string, handler http.Handler, secure bool)
}

type handlers struct {
	sp   services.Provider
	mgr  *view.Manager
	cert []byte
	name string
}

// Mount registers the API on ws: the read routes on every node, event
// management on the promoter, buying on the resellers. Every /api route needs
// the client certificate trustedCert (DER). name is the node's name.
func Mount(ws WebServer, sp services.Provider, mgr *view.Manager, name string, trustedCert []byte) {
	h := &handlers{sp: sp, mgr: mgr, cert: trustedCert, name: name}
	route := func(pattern string, f http.HandlerFunc) { ws.RegisterHandler(pattern, h.auth(f), true) }

	route("GET /api/events", h.list)
	route("GET /api/events/{id}", h.get)
	if name == views.PromoterNode {
		route("POST /api/events", h.create)
		route("PUT /api/events/{id}", h.update)
	} else {
		route("POST /api/events/{id}/seats/{n}", h.buy)
	}
	mountUI(ws, h)
}

// Event is an event as the API returns it.
type Event struct {
	states.Event
	// Sold counts the sold seats per reseller.
	Sold map[string]int `json:"sold"`
	// Seating lists every seat; it is only set for a single event.
	Seating []Seat `json:"seating,omitempty"`
}

// Seat is one seat of an event.
type Seat struct {
	N        int    `json:"n"`
	Sold     bool   `json:"sold"`
	Reseller string `json:"reseller,omitempty"`
}

func (h *handlers) event(id string, withSeating bool) (*Event, error) {
	e, seats, err := views.GetEvent(h.sp, id)
	if err != nil {
		return nil, err
	}
	out := &Event{Event: *e, Sold: map[string]int{}}
	for i, s := range seats {
		seat := Seat{N: i + 1}
		if s != nil {
			seat.Sold, seat.Reseller = true, s.Reseller
			out.Sold[s.Reseller]++
		}
		if withSeating {
			out.Seating = append(out.Seating, seat)
		}
	}
	return out, nil
}

func (h *handlers) events() ([]*Event, error) {
	list, err := views.ListEvents(h.sp)
	if err != nil {
		return nil, err
	}
	out := make([]*Event, 0, len(list))
	for _, e := range list {
		ev, err := h.event(e.ID, false)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func (h *handlers) list(w http.ResponseWriter, _ *http.Request) {
	out, err := h.events()
	reply(w, http.StatusOK, out, err)
}

func (h *handlers) get(w http.ResponseWriter, r *http.Request) {
	out, err := h.event(r.PathValue("id"), true)
	reply(w, http.StatusOK, out, err)
}

func (h *handlers) create(w http.ResponseWriter, r *http.Request) {
	in := states.Event{}
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.mgr.InitiateView(r.Context(), &views.CreateEventView{Event: in})
	reply(w, http.StatusCreated, out, err)
}

func (h *handlers) update(w http.ResponseWriter, r *http.Request) {
	in := states.Event{}
	if !readJSON(w, r, &in) {
		return
	}
	in.ID = r.PathValue("id")
	out, err := h.mgr.InitiateView(r.Context(), &views.UpdateEventView{Event: in})
	reply(w, http.StatusOK, out, err)
}

func (h *handlers) buy(w http.ResponseWriter, r *http.Request) {
	in, err := parseBuy(r.PathValue("id"), r.PathValue("n"), r.URL.Query().Get("hold"))
	if err != nil {
		reply(w, 0, nil, err)
		return
	}
	out, err := h.mgr.InitiateView(r.Context(), &views.BuyView{Buy: in})
	reply(w, http.StatusCreated, out, err)
}

// parseBuy checks the seat and the hold before any view runs.
func parseBuy(eventID, seat, hold string) (views.Buy, error) {
	n, err := strconv.Atoi(seat)
	if err != nil {
		return views.Buy{}, errors.Wrap(views.ErrInvalidInput, "seat must be a number")
	}
	if n < 1 || n > states.MaxSeats {
		return views.Buy{}, errors.Wrapf(views.ErrUnknownEvent, "no seat %d", n)
	}
	in := views.Buy{EventID: eventID, Seat: n}
	if hold != "" {
		if in.Hold, err = time.ParseDuration(hold); err != nil || in.Hold < 0 || in.Hold > views.MaxHold {
			return views.Buy{}, errors.Wrapf(views.ErrInvalidInput, "hold must be a duration up to %s", views.MaxHold)
		}
	}
	return in, nil
}

// maxBody bounds a request body: an event is a few hundred bytes.
const maxBody = 16 << 10

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body is not a valid event: "+err.Error())
		return false
	}
	return true
}

func reply(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		code, name := statusOf(err)
		writeError(w, code, name, err.Error())
		return
	}
	writeJSON(w, status, v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
