/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"testing"

	"github.com/stretchr/testify/require"

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
	require.Equal(t, sess, s.Get("v1", party))

	s.Delete("v1", party)
	require.Nil(t, s.Get("v1", party))

	s.PutDefault(party, sess)
	require.Equal(t, sess, s.Get("", party))

	require.Equal(t, []view.SessionInfo{{ID: "s1"}}, s.GetSessionInfos())

	s.Reset()
	require.Empty(t, s.GetSessionInfos())
}
