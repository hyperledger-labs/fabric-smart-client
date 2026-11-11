/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewTestLogger_ContextMethods checks that every level-guarded *Context method of the
// test logger records exactly one entry, at its level, with the message formatted.
func TestNewTestLogger_ContextMethods(t *testing.T) {
	t.Parallel()
	for _, m := range contextMethods() {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()
			l, r := NewTestLogger(t)

			m.call(l, t.Context())

			entries := r.Entries()
			require.Len(t, entries, 1)
			want := "hello world"
			if m.structured {
				want = "hello"
			}
			require.Equal(t, []string{want}, r.Messages())
			require.Contains(t, entries[0], " "+strings.ToUpper(m.level.String())[:4]+" ")
		})
	}
}

func TestNewTestLogger_StructuredContextFields(t *testing.T) {
	t.Parallel()
	l, r := NewTestLogger(t)

	l.InfowContext(t.Context(), "hello", "k", "v")

	entries := r.Entries()
	require.Len(t, entries, 1)
	require.True(t, strings.HasSuffix(entries[0], "hello k=v"), entries[0])
}

// TestNewTestLogger_Caller checks that entries report the function that called the log
// method, for plain and *Context calls, on the logger and on loggers derived from it. The
// recorder's format prints that function's short name as "<name> ->".
func TestNewTestLogger_Caller(t *testing.T) {
	t.Parallel()
	base, r := NewTestLogger(t)

	for _, l := range []Logger{base, base.Named("child"), base.With("k", "v"), base.Named("a").With("k", "v").Named("b")} {
		l.Infof("plain")
		l.InfofContext(t.Context(), "ctx")
	}

	entries := r.Entries()
	require.Len(t, entries, 8)
	for _, e := range entries {
		require.Contains(t, e, " TestNewTestLogger_Caller -> ")
	}
}
