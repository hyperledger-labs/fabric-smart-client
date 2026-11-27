/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chat

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// loggedIn returns Rooms for org1 and org2 with alice, bob (org1) and charlie, dave (org2)
// logged in.
func loggedIn(t *testing.T) *Rooms {
	t.Helper()
	r := NewRooms([]string{"org1", "org2"})
	for name, org := range map[string]string{"alice": "org1", "bob": "org1", "charlie": "org2", "dave": "org2"} {
		r.Login(name, org, view.Identity(name))
	}
	return r
}

func recipients(t *testing.T, r *Rooms, name, channel string) []string {
	t.Helper()
	ids, err := r.Recipients(name, channel)
	require.NoError(t, err)
	return slices.Sorted(maps.Keys(ids))
}

func TestJoinOrgChannel(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)

	require.NoError(t, r.Join("alice", "org1"))
	require.ErrorIs(t, r.Join("charlie", "org1"), ErrOrgOnly)
	require.ErrorIs(t, r.Join("charlie", "Org1"), ErrOrgOnly)
	require.NoError(t, r.Join("charlie", "general"))
	require.NoError(t, r.Join("bob", "ORG1"))
	require.Equal(t, []string{"bob"}, recipients(t, r, "alice", "org1"))
}

func TestJoinRequiresLogin(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)

	require.ErrorIs(t, r.Join("eve", "general"), ErrNotOnline)
}

func TestRecipients(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)
	for _, name := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, r.Join(name, "general"))
	}

	require.Equal(t, []string{"bob", "charlie"}, recipients(t, r, "alice", "general"))
	ids, err := r.Recipients("alice", "general")
	require.NoError(t, err)
	require.Equal(t, view.Identity("bob"), ids["bob"])
	_, err = r.Recipients("dave", "general")
	require.ErrorIs(t, err, ErrNotMember)
	_, err = r.Recipients("alice", "nowhere")
	require.ErrorIs(t, err, ErrNotMember)
}

func TestRecipientsIsSnapshot(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)
	require.NoError(t, r.Join("alice", "general"))
	require.NoError(t, r.Join("bob", "general"))

	ids, err := r.Recipients("alice", "general")
	require.NoError(t, err)
	delete(ids, "bob")
	ids["mallory"] = view.Identity("mallory")

	require.Equal(t, []string{"bob"}, recipients(t, r, "alice", "general"))
}

func TestLeaveAndLogout(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)
	for _, name := range []string{"alice", "bob", "charlie"} {
		require.NoError(t, r.Join(name, "general"))
	}

	require.NoError(t, r.Leave("bob", "general"))
	require.Equal(t, []string{"charlie"}, recipients(t, r, "alice", "general"))
	require.ErrorIs(t, r.Leave("bob", "general"), ErrNotMember)

	r.Logout("charlie")
	require.Empty(t, recipients(t, r, "alice", "general"))
	require.Equal(t, []User{{"alice", "org1"}, {"bob", "org1"}, {"dave", "org2"}}, r.Who())
}

func TestLoginReplaces(t *testing.T) {
	t.Parallel()
	r := loggedIn(t)
	require.NoError(t, r.Join("alice", "general"))
	require.NoError(t, r.Join("bob", "general"))

	users := r.Login("alice", "org1", view.Identity("alice-restarted"))

	require.Equal(t, []User{{"alice", "org1"}, {"bob", "org1"}, {"charlie", "org2"}, {"dave", "org2"}}, users)
	require.Empty(t, recipients(t, r, "bob", "general"))
	require.NoError(t, r.Join("alice", "general"))
	ids, err := r.Recipients("bob", "general")
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-restarted"), ids["alice"])
}
