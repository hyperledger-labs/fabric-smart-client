/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import "sync"

type entry struct {
	Wrapped interface{}
	Wrapper interface{}
}

// Subscribers is a thread-safe map of subscribers.
// It is used to keep track of bindings between wrapped listeners and their wrappers.
type Subscribers struct {
	backend map[string][]*entry
	mutex   sync.RWMutex
}

// NewSubscribers returns a new instance of Subscribers.
func NewSubscribers() *Subscribers {
	return &Subscribers{
		backend: map[string][]*entry{},
	}
}

// Set sets a new binding between a wrapped listener and its wrapper.
// The binding is indexed by the passed id.
func (s *Subscribers) Set(id string, wrapped, wrapper interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.backend[id] = append(s.backend[id], &entry{Wrapped: wrapped, Wrapper: wrapper})
}

// Get returns the wrapper listener for the given id and wrapped listener.
func (s *Subscribers) Get(id string, wrapped interface{}) (interface{}, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	list, ok := s.backend[id]
	if !ok {
		return nil, false
	}
	for _, e := range list {
		if e.Wrapped == wrapped {
			return e.Wrapper, true
		}
	}
	return nil, false
}

// Delete removes the binding for the given id and wrapped listener
func (s *Subscribers) Delete(id string, wrapped interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	list, ok := s.backend[id]
	if !ok {
		return
	}
	for i, e := range list {
		if e.Wrapped == wrapped {
			s.backend[id] = append(list[:i], list[i+1:]...)
			return
		}
	}
}
