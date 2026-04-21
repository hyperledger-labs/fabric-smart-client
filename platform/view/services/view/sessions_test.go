/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type simpleSession struct {
	view.Session
	id string
}

func (s *simpleSession) Info() view.SessionInfo {
	return view.SessionInfo{ID: s.id}
}

func TestSessions(t *testing.T) {
	t.Parallel()
	s := newSessions()
	party := view.Identity("alice")
	sess := &simpleSession{id: "s1"}

	s.Put("v1", party, sess)
	assert.Equal(t, sess, s.Get("v1", party))

	s.Delete("v1", party)
	assert.Nil(t, s.Get("v1", party))

	s.PutDefault(party, sess)
	assert.Equal(t, sess, s.Get("", party))

	ids := s.GetSessionIDs()
	assert.Len(t, ids, 1)
	assert.Equal(t, "s1", ids[0])

	s.Reset()
	assert.Empty(t, s.GetSessionIDs())
}
