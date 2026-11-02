/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package preamble

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	raw := LocalBytes()
	p, err := decode(raw)
	require.NoError(t, err)
	require.True(t, proto.Equal(Local(), p))
	require.NoError(t, checkEpoch(p))
}

func TestDecodeRefusesOversized(t *testing.T) {
	t.Parallel()
	_, err := decode(make([]byte, MaxSize+1))
	require.ErrorContains(t, err, "exceeds")

	long := strings.Repeat("x", MaxStringSize+1)
	for name, p := range map[string]*Preamble{
		"version":   {FscVersion: long},
		"revision":  {VcsRevision: long},
		"component": {Ranges: []*ComponentRange{{Component: long}}},
	} {
		raw, err := proto.Marshal(p)
		require.NoError(t, err)
		_, err = decode(raw)
		require.ErrorContains(t, err, "string field exceeds", name)
		_, err = Encode(p)
		require.ErrorContains(t, err, "string field exceeds", name)
	}
}

// A field a later release adds is ignored, so preambles of every release stay readable.
func TestDecodeIgnoresUnknownFields(t *testing.T) {
	t.Parallel()
	raw := protowire.AppendTag(LocalBytes(), 99, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte("from a later release"))
	p, err := decode(raw)
	require.NoError(t, err)
	require.Equal(t, FramingEpoch, p.GetFramingEpoch())
}

func TestCheckEpoch(t *testing.T) {
	t.Parallel()
	err := checkEpoch(&Preamble{FramingEpoch: FramingEpoch + 1})
	require.ErrorIs(t, err, ErrEpoch)
	require.ErrorContains(t, err, "peer framing epoch 2, local 1")
}
