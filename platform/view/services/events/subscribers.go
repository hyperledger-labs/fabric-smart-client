/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package events

import "sync"

type Entry struct {
	Parent interface{}
	Child  interface{}
}

type Subscribers struct {
	backend map[string][]*Entry
	mutex   sync.RWMutex
}

func NewSubscribers() *Subscribers {
	return &Subscribers{
		backend: map[string][]*Entry{},
	}
}

func (s *Subscribers) Store(id string, parent, child interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.backend[id] = append(s.backend[id], &Entry{Parent: parent, Child: child})
}

func (s *Subscribers) Load(id string, parent interface{}) (interface{}, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	list, ok := s.backend[id]
	if !ok {
		return nil, false
	}
	for _, e := range list {
		if e.Parent == parent {
			return e.Child, true
		}
	}
	return nil, false
}

func (s *Subscribers) Delete(id string, parent interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	list, ok := s.backend[id]
	if !ok {
		return
	}
	for i, e := range list {
		if e.Parent == parent {
			s.backend[id] = append(list[:i], list[i+1:]...)
			return
		}
	}
}
