# Ticket booking on Chaguar

This example is for developers who run an existing system, such as a web shop
or a back office, and want to connect it to a ledger that several organizations
share. It sells concert tickets. A promoter creates events, and two resellers
in different regions sell seats of the same events to their own customers.
Each organization runs one FSC node. Each node serves a small JSON REST API and
a web page, and does the multi-party work and the ledger work behind them. The
existing system only makes HTTP calls.

No central ticketing company sits between the resellers. What stops a seat
from being sold twice is the shared ledger, and the example shows it happening:
when both resellers sell the same seat at the same moment, one of them gets
`409 lost_race`.

For the view programming model itself, read the
[Programming model guide][programming-model] first.

[programming-model]: ../../docs/platform/view/programming-model.md

## The parties

| FSC node      | Organization | Role                                              |
| ------------- | ------------ | ------------------------------------------------- |
| `promoter`    | `Promoter`   | creates and edits events; endorses every sale     |
| `reseller-eu` | `ResellerEU` | sells seats; its customers stay in its own system |
| `reseller-us` | `ResellerUS` | sells seats; its customers stay in its own system |

The `tickets` namespace has the endorsement policy `AND('PromoterMSP.member')`
([`topology.go`](topology.go)): no transaction commits without the promoter's
signature. Chaguar runs no chaincode, so the promoter's endorsement is the only
place where the rules of a sale are checked. Those rules are plain Go in
[`views/rules.go`](views/rules.go), with unit tests next to them.

Customers have no node and no ledger identity. Payment and customer accounts
stay in the reseller's existing system. A sale returns a random ticket
reference, and the reseller's system keeps which customer holds it. The ledger
never sees a name or an email address: every organization reads it, which
makes it the wrong place for personal data.

## What is on the ledger

| Key             | Written by | Holds                                                         |
| --------------- | ---------- | ------------------------------------------------------------- |
| `events`        | promoter   | the list of event IDs                                         |
| `event/<id>`    | promoter   | name, artist, venue, date, number of seats, price             |
| `seat/<id>/<n>` | a reseller | which reseller sold seat `n`, the price, the ticket reference |

A seat key is written once, when the seat is sold, and never changes.

Chaguar's query service reads a list of keys you name and has no range queries.
So every key here can be computed. The event list is a single key. An event's
seats are numbered `1` to `seats`, and the availability of a whole event is one
batched read of all its seat keys ([`views/ledger.go`](views/ledger.go)). Every
node reads the ledger directly: a reseller never asks the promoter which events
exist. Only the promoter writes `events`, and it creates one event at a time,
so two creates never conflict on the key.

## Selling a seat

[`views/reseller.go`](views/reseller.go) and
[`views/promoter.go`](views/promoter.go) hold the two sides:

1. The reseller's API receives `POST /api/events/{id}/seats/{n}`.
1. `BuyView` builds a transaction. It reads `event/<id>`, so the sale is tied
   to the price it charges, and reads `seat/<id>/<n>`, which must not exist
   yet. Then it writes the seat.
1. It sends the transaction to the promoter over an FSC session.
   `SaleResponderView` reads the event and the seat from the ledger itself,
   never from the transaction. It checks the rules, including that the
   transaction read the missing seat key. Without that read the ledger would
   accept the write as an overwrite of a seat someone else sold. Then it
   endorses.
1. The reseller submits the transaction and waits until it is committed.

The promoter keeps no seat locks. If two resellers ask for the same seat at
the same time, it endorses both. Both transactions read the seat while it did
not exist. The ledger commits the first, and rejects the second because its
read is out of date.

## Two layers stop a double sale

| Layer                                              | Error code     |
| -------------------------------------------------- | -------------- |
| the reseller or the promoter sees the seat is sold | `already_sold` |
| the ledger rejects the second sale at commit       | `lost_race`    |

When two people click in two browsers, the first sale has usually committed
before the second request even reaches the promoter, so the first layer
catches it. To watch the ledger do its job, a buy takes a demo-only `hold`: a
pause between the promoter's endorsement and the submission, at most 10
seconds. The web page has a "Hold 5 s" checkbox for it. Buy the same seat in
both reseller windows within 5 seconds and one of them reports `lost_race`.
The integration suite uses the same parameter to test the commit path.

## The REST API

The handlers are in [`api/api.go`](api/api.go). Each is a few lines: decode
the request, run a view in the same process with the node's view manager,
encode the result.

```go
func (h *handlers) buy(w http.ResponseWriter, r *http.Request) {
	in, err := parseBuy(r.PathValue("id"), r.PathValue("n"), r.URL.Query().Get("hold"))
	if err != nil {
		reply(w, 0, nil, err)
		return
	}
	out, err := h.mgr.InitiateView(r.Context(), &views.BuyView{Buy: in})
	reply(w, http.StatusCreated, out, err)
}
```

The demo's SDK ([`sdk.go`](sdk.go)) mounts them on the node's web listener,
`fsc.web`, with `WebServer.RegisterHandler`. The node's health and metrics
endpoints are mounted the same way.

| Route                             | Node      | Result                    |
| --------------------------------- | --------- | ------------------------- |
| `GET /api/events`                 | all       | events with sold counts   |
| `GET /api/events/{id}`            | all       | one event with every seat |
| `POST /api/events`                | promoter  | `201` and the new event   |
| `PUT /api/events/{id}`            | promoter  | the updated event         |
| `POST /api/events/{id}/seats/{n}` | resellers | `201` and the sold seat   |

A `504 pending` means the reseller submitted the sale but waiting for the
ledger's decision failed, so it may still commit. Its message holds the ticket
reference: look the seat up before selling it again.

An event's number of seats is fixed when it is created. A price change while a
sale is waiting to commit makes that sale fail, because the sale read the old
version of the event.

What goes wrong in a view becomes an HTTP status your system already handles,
in one function ([`api/errors.go`](api/errors.go)). Error bodies are
`{"error": "<code>", "message": "<text>"}`.

| What happened                                  | Status | Code           |
| ---------------------------------------------- | ------ | -------------- |
| bad input                                      | 400    | `bad_request`  |
| no client certificate                          | 401    |                |
| client certificate not trusted by this node    | 403    | `forbidden`    |
| unknown event, or no such seat                 | 404    | `not_found`    |
| seat already sold                              | 409    | `already_sold` |
| lost the race at commit                        | 409    | `lost_race`    |
| the event changed while the sale waited        | 409    | `conflict`     |
| the promoter refused the sale for another rule | 422    | `refused`      |
| the promoter could not be reached              | 503    | `unavailable`  |
| the sale was submitted, the outcome is unknown | 504    | `pending`      |

### Who may call the API

Every `/api` route is registered as secure, so a request needs a client
certificate from a CA the listener trusts; without one it gets `401`. In this
example's network every FSC node gets its TLS certificate from one shared CA, so
that check alone would let `reseller-us` buy through `reseller-eu`'s API. The
handlers therefore also accept only one certificate: the node's own TLS
certificate, which stands for the existing system of that organization
([`api/auth.go`](api/auth.go)). Any other certificate gets `403`. In production
each organization would issue client certificates from its own CA.

These checks cover `/api` only. The web page's routes on the same port have no
login, so anyone who reaches the port can create events or buy seats through
them (see [The web page](#the-web-page)).

### Why not the generic view API

Every FSC node can also serve its views over HTTP, at
`/v1/ViewService/ProcessCommand`. That API needs no handler code, but each
request is a signed protobuf command carrying the caller's FSC identity. That
suits FSC clients, but an existing web shop can't call it with `curl`, so this
example serves its own routes.

## The web page

Each node serves its own page at `/`: the promoter's has the event form and the
sold counts, and a reseller's has the seat grid. The pages are Go
`html/template`, styled with [Beer CSS][beercss] (Material Design 3), and call
the same views as the API ([`api/ui.go`](api/ui.go)). The stylesheet is
embedded in the binary, so the page works offline. After a buy, the page shows
the equivalent REST call and its response.

The page has no login. Anyone who can reach the port can use it. Its forms
accept posts only from the page itself (Go's `http.CrossOriginProtection`), so
another web site you visit can't submit them. In production, put the page
behind your existing system's login, or leave it out.

[beercss]: https://www.beercss.com

## Running the example

The integration suite starts Chaguar and the three nodes, drives the REST API,
and loads each node's page:

```bash
make integration-tests-examples-tickets
```

To click through it yourself, start the same network and keep it running:

```bash
make run-examples-tickets
```

It prints each node's URL and a `curl` command with that node's certificate.
Open the three URLs in three windows. The browser warns about the certificate:
it comes from the CA the test network generates. Ctrl-C stops the network.
