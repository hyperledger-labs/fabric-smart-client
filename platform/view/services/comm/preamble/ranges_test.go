/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package preamble

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHighest(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		a, b Range
		want uint32
		ok   bool
	}{
		"overlap picks the highest common": {Range{1, 3}, Range{2, 5}, 3, true},
		"one version in common":            {Range{1, 2}, Range{2, 2}, 2, true},
		"same range":                       {Range{1, 1}, Range{1, 1}, 1, true},
		"disjoint":                         {Range{1, 1}, Range{2, 3}, 0, false},
	} {
		v, ok := Highest(tc.a, tc.b)
		require.Equal(t, tc.ok, ok, name)
		if ok {
			require.Equal(t, tc.want, v, name)
		}
	}
}

func TestPeerRange(t *testing.T) {
	t.Parallel()
	p := &Preamble{Ranges: []*ComponentRange{{Component: "iou", Min: 1, Max: 2}}}
	require.Equal(t, Range{1, 2}, PeerRange(p, "iou"))
	require.Equal(t, DefaultRange, PeerRange(p, "view"), "a component the peer does not list")
	require.Equal(t, DefaultRange, PeerRange(nil, "iou"), "a peer without a preamble")
}

func TestFSCComponents(t *testing.T) {
	t.Parallel()
	require.Equal(t, []Component{
		{Name: "view", Range: Range{1, 1}},
		{Name: "fabric", Range: Range{1, 2}},
		{Name: "fabricx", Range: Range{1, 2}},
	}, FSCComponents)
}

// TestSetComponents changes process-wide state, so it runs alone and restores the default.
func TestSetComponents(t *testing.T) { //nolint:paralleltest
	t.Cleanup(func() { require.NoError(t, SetComponents(FSCComponents)) })

	require.NoError(t, SetComponents(append(FSCComponents, Component{Name: "iou", Range: Range{1, 2}})))
	r, ok := LocalRange("iou")
	require.True(t, ok)
	require.Equal(t, Range{1, 2}, r)
	p, err := decode(LocalBytes())
	require.NoError(t, err)
	require.Equal(t, Range{1, 2}, PeerRange(p, "iou"), "the preamble publishes the components")

	err = SetComponents([]Component{{Name: "iou", Range: Range{1, 1}}, {Name: "iou", Range: Range{2, 3}}})
	require.EqualError(t, err, "component [iou] registered twice: [1, 1] and [2, 3]")
	for _, bad := range []Component{{Name: "Iou", Range: Range{1, 1}}, {Name: "a/b", Range: Range{1, 1}}, {Name: "iou", Range: Range{0, 1}}, {Name: "iou", Range: Range{2, 1}}} {
		require.Error(t, SetComponents([]Component{bad}), bad)
	}
	_, ok = LocalRange("iou")
	require.True(t, ok, "a failed SetComponents keeps the components")
}

// TestRegisterComponent changes process-wide state, so it runs alone and restores the default.
func TestRegisterComponent(t *testing.T) { //nolint:paralleltest
	t.Cleanup(func() { require.NoError(t, SetComponents(FSCComponents)) })

	RegisterComponent("iou", 1, 2)
	r, ok := LocalRange("iou")
	require.True(t, ok)
	require.Equal(t, Range{1, 2}, r)
	require.Len(t, Components(), len(FSCComponents)+1, "FSC's components stay")
	require.PanicsWithError(t, "component [iou] registered twice: [1, 2] and [2, 3]", func() { RegisterComponent("iou", 2, 3) })
	require.Panics(t, func() { RegisterComponent("view", 1, 2) }, "an FSC component cannot be registered again")
	require.Panics(t, func() { RegisterComponent("Bad/Name", 1, 1) })
}
