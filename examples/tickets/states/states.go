/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package states holds the ledger states of the ticket booking demo and the
// keys they live under.
//
// Chaguar's query service reads named keys only: it has no range queries. So
// every key here can be computed: the event list is one key, an event's seats
// are numbered from 1 to Seats.
package states

import (
	"strconv"
	"strings"
)

const (
	// EventsKey holds the JSON list of every event's ID. Only the promoter
	// writes it, in the transaction that creates an event.
	EventsKey = "events"
	// MaxSeats bounds an event's seats, so reading all of them stays one small
	// batched query.
	MaxSeats = 200
)

// Event is a concert or show the promoter sells seats for. Only the promoter
// writes it. Seats never changes after the event is created.
type Event struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Artist string `json:"artist"`
	Venue  string `json:"venue"`
	// Date is the day of the event, in YYYY-MM-DD form.
	Date  string `json:"date"`
	Seats int    `json:"seats"`
	// Price is what one seat costs, in the smallest unit of the currency.
	Price uint64 `json:"price"`
}

// Seat is a sold seat. It is written once, when the seat is sold, and never
// changes. It holds no customer data: the reseller's own system keeps which
// customer holds which Ticket.
type Seat struct {
	Event string `json:"event"`
	N     int    `json:"seat"`
	// Reseller is the FSC node that sold the seat.
	Reseller string `json:"reseller"`
	Price    uint64 `json:"price"`
	// Ticket is a random reference the reseller hands to its customer.
	Ticket string `json:"ticket"`
}

// EventKey is where the event with the given ID lives.
func EventKey(id string) string { return "event/" + id }

// SeatKey is where seat n of the event is written when it is sold.
func SeatKey(eventID string, n int) string { return "seat/" + eventID + "/" + strconv.Itoa(n) }

// ParseSeatKey splits a seat key into its event ID and seat number. ok is false
// for any key SeatKey would not produce.
func ParseSeatKey(key string) (eventID string, n int, ok bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "seat" || parts[1] == "" {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n < 1 || strconv.Itoa(n) != parts[2] {
		return "", 0, false
	}
	return parts[1], n, true
}
