/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

func TestSeatKeyRoundTrip(t *testing.T) {
	t.Parallel()
	id, n, ok := states.ParseSeatKey(states.SeatKey("ab12", 7))
	require.True(t, ok)
	require.Equal(t, "ab12", id)
	require.Equal(t, 7, n)

	for _, key := range []string{"event/ab12", "seat/ab12/x", "seat/ab12/0", "seat//3", "seat/a/b/3"} {
		_, _, ok := states.ParseSeatKey(key)
		require.False(t, ok, key)
	}
}

func validEvent() *states.Event {
	return &states.Event{ID: "e1", Name: "Neon Nights", Artist: "Neon", Venue: "Berlin", Date: "2027-03-02", Seats: 40, Price: 50}
}

func TestValidateEvent(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateEvent(validEvent()))

	for name, mutate := range map[string]func(*states.Event){
		"empty name":   func(e *states.Event) { e.Name = "" },
		"empty artist": func(e *states.Event) { e.Artist = "" },
		"long venue":   func(e *states.Event) { e.Venue = strings.Repeat("v", 101) },
		"no seats":     func(e *states.Event) { e.Seats = 0 },
		"too many":     func(e *states.Event) { e.Seats = 201 },
		"no price":     func(e *states.Event) { e.Price = 0 },
		"bad date":     func(e *states.Event) { e.Date = "02.03.2027" },
	} {
		e := validEvent()
		mutate(e)
		require.ErrorIs(t, ValidateEvent(e), ErrInvalidInput, name)
	}
}

func seatValue(t *testing.T, s states.Seat) []byte {
	t.Helper()
	raw, err := json.Marshal(s)
	require.NoError(t, err)
	return raw
}

func validSale(t *testing.T) SaleTx {
	t.Helper()
	return SaleTx{
		Reads: map[string][]byte{
			states.EventKey("e1"):   []byte("v1"),
			states.SeatKey("e1", 8): nil,
		},
		Writes: map[string][]byte{
			states.SeatKey("e1", 8): seatValue(t, states.Seat{Event: "e1", N: 8, Reseller: "reseller-eu", Price: 50, Ticket: "t"}),
		},
	}
}

func TestCheckSaleAcceptsValidSale(t *testing.T) {
	t.Parallel()
	seat, err := CheckSale(validSale(t), validEvent(), false, "reseller-eu")
	require.NoError(t, err)
	require.Equal(t, &states.Seat{Event: "e1", N: 8, Reseller: "reseller-eu", Price: 50, Ticket: "t"}, seat)
}

func TestCheckSaleRejects(t *testing.T) {
	t.Parallel()
	withSeat := func(s states.Seat) func(*SaleTx) {
		return func(tx *SaleTx) { tx.Writes[states.SeatKey("e1", 8)] = seatValue(t, s) }
	}
	cases := []struct {
		name   string
		event  *states.Event
		sold   bool
		mutate func(*SaleTx)
		want   error
	}{
		{name: "unknown event", want: ErrUnknownEvent},
		{name: "seat sold", event: validEvent(), sold: true, want: ErrSeatSold},
		{
			name: "blind write", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) { delete(tx.Reads, states.SeatKey("e1", 8)) },
		},
		{
			name: "seat read with a version", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) { tx.Reads[states.SeatKey("e1", 8)] = []byte("v2") },
		},
		{
			name: "event not read", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) { delete(tx.Reads, states.EventKey("e1")) },
		},
		{
			name: "two writes", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) { tx.Writes[states.SeatKey("e1", 9)] = []byte("{}") },
		},
		{
			name: "writes the event", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) {
				tx.Writes = map[string][]byte{states.EventKey("e1"): []byte("{}")}
			},
		},
		{
			name: "seat out of range", event: validEvent(), want: ErrUnknownEvent,
			mutate: func(tx *SaleTx) {
				tx.Reads = map[string][]byte{states.EventKey("e1"): []byte("v1"), states.SeatKey("e1", 41): nil}
				tx.Writes = map[string][]byte{states.SeatKey("e1", 41): seatValue(t, states.Seat{Event: "e1", N: 41, Reseller: "reseller-eu", Price: 50, Ticket: "t"})}
			},
		},
		{
			name: "other reseller", event: validEvent(), want: ErrRefused,
			mutate: withSeat(states.Seat{Event: "e1", N: 8, Reseller: "reseller-us", Price: 50, Ticket: "t"}),
		},
		{
			name: "wrong price", event: validEvent(), want: ErrRefused,
			mutate: withSeat(states.Seat{Event: "e1", N: 8, Reseller: "reseller-eu", Price: 40, Ticket: "t"}),
		},
		{
			name: "no ticket", event: validEvent(), want: ErrRefused,
			mutate: withSeat(states.Seat{Event: "e1", N: 8, Reseller: "reseller-eu", Price: 50}),
		},
		{
			name: "seat number mismatch", event: validEvent(), want: ErrRefused,
			mutate: withSeat(states.Seat{Event: "e1", N: 9, Reseller: "reseller-eu", Price: 50, Ticket: "t"}),
		},
		{
			name: "event mismatch", event: validEvent(), want: ErrRefused,
			mutate: withSeat(states.Seat{Event: "e2", N: 8, Reseller: "reseller-eu", Price: 50, Ticket: "t"}),
		},
		{
			name: "not JSON", event: validEvent(), want: ErrRefused,
			mutate: func(tx *SaleTx) { tx.Writes[states.SeatKey("e1", 8)] = []byte("nope") },
		},
	}
	for _, c := range cases {
		tx := validSale(t)
		if c.mutate != nil {
			c.mutate(&tx)
		}
		_, err := CheckSale(tx, c.event, c.sold, "reseller-eu")
		require.True(t, errors.Is(err, c.want), "%s: got %v", c.name, err)
	}
}
