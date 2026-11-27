/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package views holds the ticket demo's FSC views and the rules the promoter
// checks before it endorses a sale.
package views

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const (
	// Namespace is where the demo's states live on the ledger.
	Namespace = "tickets"
	// PromoterNode is the name of the promoter's FSC node.
	PromoterNode = "promoter"

	maxTextLen = 100
)

// The errors the views return. The REST API turns each into an HTTP status.
var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnknownEvent = errors.New("unknown event or seat")
	// ErrSeatSold's text travels inside the promoter's refusal, so the reseller
	// can tell it apart from other refusals.
	ErrSeatSold = errors.New("seat already sold")
	// ErrLostRace means the promoter endorsed the sale, but another reseller's
	// sale of the same seat committed first and the ledger rejected this one.
	ErrLostRace = errors.New("lost the race")
	// ErrConflict means the sale failed at commit for another reason, such as
	// a price change while it waited.
	ErrConflict    = errors.New("conflict")
	ErrRefused     = errors.New("promoter refused the sale")
	ErrUnreachable = errors.New("promoter unreachable")
	// ErrPending means the sale was submitted, but waiting for the ledger's
	// decision failed: it may still commit. Its message holds the ticket
	// reference, so the caller can look the seat up later.
	ErrPending = errors.New("sale submitted, outcome unknown")
)

// ValidateEvent checks an event the promoter is about to create or update.
func ValidateEvent(e *states.Event) error {
	for field, v := range map[string]string{"name": e.Name, "artist": e.Artist, "venue": e.Venue} {
		if v == "" || len(v) > maxTextLen {
			return errors.Wrapf(ErrInvalidInput, "%s must have 1 to %d bytes", field, maxTextLen)
		}
	}
	if _, err := time.Parse(time.DateOnly, e.Date); err != nil {
		return errors.Wrapf(ErrInvalidInput, "date [%s] is not YYYY-MM-DD", e.Date)
	}
	if e.Seats < 1 || e.Seats > states.MaxSeats {
		return errors.Wrapf(ErrInvalidInput, "seats must be 1 to %d", states.MaxSeats)
	}
	if e.Price == 0 {
		return errors.Wrap(ErrInvalidInput, "price must be positive")
	}
	return nil
}

// SaleTx is what the promoter looks at in a buy transaction: its reads in the
// demo's namespace (key to read version, nil for a key read while it did not
// exist) and its writes (key to value).
type SaleTx struct {
	Reads  map[string][]byte
	Writes map[string][]byte
}

// CheckSale plays the part a chaincode plays in Fabric: Chaguar runs no
// contract code, so the promoter's endorsement is the only check a sale gets.
// event and seatSold come from the ledger, never from the transaction; caller
// is the FSC node that asked for the endorsement. It returns the seat sold.
func CheckSale(tx SaleTx, event *states.Event, seatSold bool, caller string) (*states.Seat, error) {
	if event == nil {
		return nil, ErrUnknownEvent
	}
	if len(tx.Writes) != 1 {
		return nil, errors.Wrapf(ErrRefused, "a sale writes one seat, got %d writes", len(tx.Writes))
	}
	var key string
	var value []byte
	for k, v := range tx.Writes {
		key, value = k, v
	}
	eventID, n, ok := states.ParseSeatKey(key)
	if !ok || eventID != event.ID {
		return nil, errors.Wrapf(ErrRefused, "write [%s] is not a seat of event [%s]", key, event.ID)
	}
	if n > event.Seats {
		return nil, errors.Wrapf(ErrUnknownEvent, "event [%s] has %d seats, not %d", event.ID, event.Seats, n)
	}
	if seatSold {
		return nil, errors.Wrapf(ErrSeatSold, "seat %d of event [%s]", n, event.ID)
	}
	// Without a read of the missing seat, the ledger would let the write
	// overwrite a seat that someone else sold in the meantime.
	if version, read := tx.Reads[key]; !read || version != nil {
		return nil, errors.Wrapf(ErrRefused, "a sale must read seat [%s] while it does not exist", key)
	}
	// Reading the event ties the price to the event's committed version.
	if _, read := tx.Reads[states.EventKey(event.ID)]; !read {
		return nil, errors.Wrapf(ErrRefused, "a sale must read event [%s]", event.ID)
	}

	seat := &states.Seat{}
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(seat); err != nil {
		return nil, errors.Wrapf(ErrRefused, "seat [%s] is not a valid seat: %s", key, err)
	}
	switch {
	case seat.Event != event.ID || seat.N != n:
		return nil, errors.Wrapf(ErrRefused, "seat value does not match key [%s]", key)
	case seat.Reseller != caller:
		return nil, errors.Wrapf(ErrRefused, "seat names reseller [%s], but [%s] is selling it", seat.Reseller, caller)
	case seat.Price != event.Price:
		return nil, errors.Wrapf(ErrRefused, "seat price %d is not the event's price %d", seat.Price, event.Price)
	case seat.Ticket == "":
		return nil, errors.Wrap(ErrRefused, "seat has no ticket reference")
	}
	return seat, nil
}
