/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tickets/states"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
)

// Reads go straight to the ledger, through Chaguar's query service. It reads a
// list of named keys in one call and has no range queries, which is why every
// key in package states can be computed.

// readStates returns the committed values of keys; a key that does not exist
// is missing from the result.
func readStates(sp services.Provider, keys ...string) (map[string][]byte, error) {
	fns, ch, err := fabric.GetDefaultChannel(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the default channel")
	}
	qs, err := queryservice.GetQueryService(sp, fns.Name(), ch.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed getting the query service")
	}
	res, err := qs.GetStates(map[driver.Namespace][]driver.PKey{Namespace: keys})
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading %d keys", len(keys))
	}
	out := make(map[string][]byte, len(res[Namespace]))
	for k, v := range res[Namespace] {
		if len(v.Raw) > 0 {
			out[k] = v.Raw
		}
	}
	return out, nil
}

// ListEvents returns every event, in the order the promoter created them.
func ListEvents(sp services.Provider) ([]*states.Event, error) {
	res, err := readStates(sp, states.EventsKey)
	if err != nil {
		return nil, err
	}
	var ids []string
	if raw, ok := res[states.EventsKey]; ok {
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, errors.Wrap(err, "failed decoding the event list")
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = states.EventKey(id)
	}
	res, err = readStates(sp, keys...)
	if err != nil {
		return nil, err
	}
	events := make([]*states.Event, 0, len(ids))
	for _, key := range keys {
		e := &states.Event{}
		if err := json.Unmarshal(res[key], e); err != nil {
			return nil, errors.Wrapf(err, "failed decoding [%s]", key)
		}
		events = append(events, e)
	}
	return events, nil
}

// GetEvent returns the event and its seats: seats[i] is seat i+1, nil while it
// is free.
func GetEvent(sp services.Provider, id string) (*states.Event, []*states.Seat, error) {
	res, err := readStates(sp, states.EventKey(id))
	if err != nil {
		return nil, nil, err
	}
	raw, ok := res[states.EventKey(id)]
	if !ok {
		return nil, nil, errors.Wrapf(ErrUnknownEvent, "event [%s]", id)
	}
	e := &states.Event{}
	if err := json.Unmarshal(raw, e); err != nil {
		return nil, nil, errors.Wrapf(err, "failed decoding event [%s]", id)
	}

	keys := make([]string, e.Seats)
	for i := range keys {
		keys[i] = states.SeatKey(id, i+1)
	}
	res, err = readStates(sp, keys...)
	if err != nil {
		return nil, nil, err
	}
	seats := make([]*states.Seat, e.Seats)
	for i, key := range keys {
		raw, ok := res[key]
		if !ok {
			continue
		}
		seats[i] = &states.Seat{}
		if err := json.Unmarshal(raw, seats[i]); err != nil {
			return nil, nil, errors.Wrapf(err, "failed decoding [%s]", key)
		}
	}
	return e, seats, nil
}
