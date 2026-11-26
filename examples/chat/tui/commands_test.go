/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]Command{
		"hello":                {Kind: KindSay, Text: "hello"},
		"  hi there ":          {Kind: KindSay, Text: "hi there"},
		"/join General":        {Kind: KindJoin, Arg: "general"},
		"/leave":               {Kind: KindLeave},
		"/leave org1":          {Kind: KindLeave, Arg: "org1"},
		"/msg dave lunch? now": {Kind: KindMsg, Arg: "dave", Text: "lunch? now"},
		"/who":                 {Kind: KindWho},
		"/quit":                {Kind: KindQuit},
	} {
		got, err := Parse(line)
		require.NoError(t, err, line)
		require.Equal(t, want, got, line)
	}
	for _, line := range []string{"", "   ", "/join", "/msg dave", "/msg", "/nope", "/join a b"} {
		_, err := Parse(line)
		require.Error(t, err, "%q", line)
	}
}
