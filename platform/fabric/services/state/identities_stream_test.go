/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestIdentitiesHelpers(t *testing.T) {
	t.Parallel()

	alice := view.Identity("alice")
	bob := view.Identity("bob")
	carol := view.Identity("carol")

	ids := Identities{alice, bob, carol}
	require.Equal(t, 3, ids.Count())

	filtered := ids.Filter(func(identity view.Identity) bool {
		return identity.Equal(alice) || identity.Equal(carol)
	})
	require.Equal(t, Identities{alice, carol}, filtered)

	others := ids.Others(bob)
	require.Equal(t, Identities{alice, carol}, others)

	require.True(t, ids.Match([]view.Identity{bob, carol, alice}))
	require.False(t, ids.Match([]view.Identity{bob, carol}))
	require.False(t, ids.Match([]view.Identity{bob, carol, view.Identity("dave")}))

	require.True(t, ids.Contain(carol))
	require.False(t, ids.Contain(view.Identity("dave")))
}

func TestIDAndIDsHelpers(t *testing.T) {
	t.Parallel()

	composite, err := CreateCompositeKey("asset", []string{"id1"})
	require.NoError(t, err)
	plainComposite, err := CreateCompositeKey("asset", nil)
	require.NoError(t, err)

	id := ID(composite)
	require.True(t, id.HasPrefix("asset"))
	require.False(t, id.HasPrefix("other"))
	require.False(t, ID("plain").HasPrefix("asset"))
	require.False(t, ID(plainComposite).HasPrefix("asset"))

	filter := IDHasPrefixFilter("asset")
	require.True(t, filter(id))
	require.False(t, filter(ID("plain")))

	keys := IDs{ID("k1"), ID("k2"), ID("k3")}
	require.Equal(t, 3, keys.Count())
	require.True(t, keys.Match(IDs{ID("k2"), ID("k3"), ID("k1")}))
	require.False(t, keys.Match(IDs{ID("k1"), ID("k2")}))
	require.False(t, keys.Match(IDs{ID("k1"), ID("k2"), ID("k4")}))

	kFiltered := keys.Filter(func(k ID) bool {
		return k == "k1" || k == "k3"
	})
	require.Equal(t, IDs{ID("k1"), ID("k3")}, kFiltered)
}

func TestNamespacesHelpers(t *testing.T) {
	t.Parallel()

	nss := Namespaces{"ns1", "ns2", "ns3"}
	require.Equal(t, 3, nss.Count())
	require.True(t, nss.Match(Namespaces{"ns2", "ns3", "ns1"}))
	require.False(t, nss.Match(Namespaces{"ns1", "ns2"}))
	require.False(t, nss.Match(Namespaces{"ns1", "ns2", "ns4"}))

	filtered := nss.Filter(func(ns string) bool {
		return ns == "ns1" || ns == "ns3"
	})
	require.Equal(t, Namespaces{"ns1", "ns3"}, filtered)
}

func TestStreamsHelpers(t *testing.T) {
	t.Parallel()

	composite1, err := CreateCompositeKey("asset", []string{"1"})
	require.NoError(t, err)
	composite2, err := CreateCompositeKey("asset", []string{"2"})
	require.NoError(t, err)
	composite3, err := CreateCompositeKey("asset", []string{"3"})
	require.NoError(t, err)

	outs := []*output{
		{key: ID(composite1), delete: false},
		{key: ID(composite2), delete: true},
		{key: ID(composite3), delete: false},
	}
	os := &outputStream{outputs: outs}
	require.Equal(t, 3, os.Count())
	require.Equal(t, outs[1], os.At(1))
	require.Equal(t, IDs{ID(composite1), ID(composite2), ID(composite3)}, os.IDs())

	deleted := os.Deleted()
	require.Equal(t, 1, deleted.Count())
	require.Equal(t, ID(composite2), deleted.At(0).ID())
	require.True(t, deleted.At(0).IsDelete())

	written := os.Written()
	require.Equal(t, 2, written.Count())
	require.False(t, written.At(0).IsDelete())
	require.False(t, written.At(1).IsDelete())

	of := os.Filter(func(o *output) bool { return o.key == ID(composite3) })
	require.Equal(t, 1, of.Count())
	require.Equal(t, ID(composite3), of.At(0).ID())

	ins := []*input{
		{key: ID(composite1), index: 0},
		{key: ID(composite2), index: 1},
	}
	is := &inputStream{inputs: ins}
	require.Equal(t, 2, is.Count())
	require.Equal(t, ins[0], is.At(0))
	require.Equal(t, IDs{ID(composite1), ID(composite2)}, is.IDs())
	require.Equal(t, ID(composite1), ins[0].ID())

	ifilter := InputHasIDPrefixFilter("asset")
	require.True(t, ifilter(ins[0]))
	require.True(t, ifilter(ins[1]))
	require.False(t, ifilter(&input{key: ID("plain")}))

	isf := is.Filter(func(i *input) bool { return i.index == 1 })
	require.Equal(t, 1, isf.Count())
	require.Equal(t, ins[1], isf.At(0))

	commands := []*Command{
		{Name: "cmd1"},
		{Name: "cmd2"},
		{Name: "cmd3"},
	}
	cs := &commandStream{commands: commands}
	require.Equal(t, 3, cs.Count())
	require.Equal(t, commands[2], cs.At(2))

	csf := cs.Filter(func(c *Command) bool { return c.Name != "cmd2" })
	require.Equal(t, 2, csf.Count())
	require.Equal(t, "cmd1", csf.At(0).Name)
	require.Equal(t, "cmd3", csf.At(1).Name)
}
