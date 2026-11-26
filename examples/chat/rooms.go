/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	// ErrNotOnline is returned for a user who is not logged in.
	ErrNotOnline = errors.New("not logged in")
	// ErrOrgOnly is returned when a user joins another organization's channel.
	ErrOrgOnly = errors.New("channel is for members of its organization only")
	// ErrNotMember is returned when a user leaves or posts to a channel it has not joined.
	ErrNotMember = errors.New("not a member of the channel")
)

type member struct {
	org string
	id  view.Identity
}

// Rooms holds who is online and who is in which channel. A channel named like an
// organization admits only that organization's members; every other channel is open.
// Channel names are case-insensitive.
type Rooms struct {
	// ponytail: one lock for all channels, fine at demo scale.
	mu       sync.Mutex
	orgs     map[string]bool
	online   map[string]member
	channels map[string]map[string]bool
}

// NewRooms returns empty Rooms whose organization channels are orgs.
func NewRooms(orgs []string) *Rooms {
	r := &Rooms{orgs: map[string]bool{}, online: map[string]member{}, channels: map[string]map[string]bool{}}
	for _, org := range orgs {
		r.orgs[strings.ToLower(org)] = true
	}
	return r
}

// Login marks name online with identity id. A user who logs in again starts over, without
// the channels it was in.
func (r *Rooms) Login(name, org string, id view.Identity) []User {
	r.mu.Lock()
	r.leaveAll(name)
	r.online[name] = member{org: org, id: id}
	r.mu.Unlock()
	return r.Who()
}

// Logout marks name offline and removes it from every channel.
func (r *Rooms) Logout(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leaveAll(name)
	delete(r.online, name)
}

// Who returns the users online, sorted by name.
func (r *Rooms) Who() []User {
	r.mu.Lock()
	defer r.mu.Unlock()
	users := make([]User, 0, len(r.online))
	for _, name := range slices.Sorted(maps.Keys(r.online)) {
		users = append(users, User{Name: name, Org: r.online[name].org})
	}
	return users
}

// Join adds name to channel, creating the channel if needed.
func (r *Rooms) Join(name, channel string) error {
	channel = strings.ToLower(channel)
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.online[name]
	if !ok {
		return ErrNotOnline
	}
	if r.orgs[channel] && m.org != channel {
		return ErrOrgOnly
	}
	if r.channels[channel] == nil {
		r.channels[channel] = map[string]bool{}
	}
	r.channels[channel][name] = true
	return nil
}

// Leave removes name from channel; a channel nobody is in is dropped.
func (r *Rooms) Leave(name, channel string) error {
	channel = strings.ToLower(channel)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.channels[channel][name] {
		return ErrNotMember
	}
	r.drop(name, channel)
	return nil
}

// Recipients returns the identities of channel's members other than name, which must be a
// member. The map is a copy: the caller sends to it without holding the lock.
func (r *Rooms) Recipients(name, channel string) (map[string]view.Identity, error) {
	channel = strings.ToLower(channel)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.channels[channel][name] {
		return nil, ErrNotMember
	}
	ids := map[string]view.Identity{}
	for other := range r.channels[channel] {
		if other != name {
			ids[other] = r.online[other].id
		}
	}
	return ids, nil
}

func (r *Rooms) leaveAll(name string) {
	for channel := range r.channels {
		r.drop(name, channel)
	}
}

func (r *Rooms) drop(name, channel string) {
	delete(r.channels[channel], name)
	if len(r.channels[channel]) == 0 {
		delete(r.channels, channel)
	}
}
