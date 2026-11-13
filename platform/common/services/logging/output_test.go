/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stringer string

func (s stringer) String() string { return "S(" + string(s) + ")" }

// writeGoldenLines writes the lines of the golden tests: fields of several kinds, With,
// Named, a disabled line, an Error line, and an Error line from slog through Handler.
func writeGoldenLines() {
	l := MustGetLogger("golden")
	l.Infow("hello", "int", 1, "str", "x", "err", context.Canceled, "stringer", stringer("v"),
		"dur", time.Second, "bytes", []byte("hi"))
	l.With("w", 2).Named("sub").Named("").Infof("with %d", 3)
	l.Debugf("dropped")
	l.Errorf("bad %s", "thing")
	slog.New(Handler("app")).Error("from slog")
}

var (
	jsonTS = regexp.MustCompile(`"ts":[0-9.e+]+`)
	// jsonStack keeps the first frame of a stack trace, which names the caller.
	jsonStack = regexp.MustCompile(`("stacktrace":"[^\\]*)[^"]*"`)
)

// TestDefaultOutput_Golden pins the lines of the default handler, in the pattern format
// and in JSON: operators parse their format and fields.
func TestDefaultOutput_Golden(t *testing.T) { //nolint:paralleltest // changes the global logging config
	const name = "fsc.platform.common.services.logging.golden"
	for _, tc := range []struct { //nolint:paralleltest // the subtests change the global logging config
		format string
		want   []string
	}{{
		format: "[%{module}] %{shortfunc} -> %{level:.4s} %{message}",
		want: []string{
			"[" + name + "] writeGoldenLines -> INFO hello int=1 str=x err=\"context canceled\" stringer=S(v) dur=1s bytes=hi",
			"[" + name + ".sub] writeGoldenLines -> INFO with 3 w=2",
			"[" + name + "] writeGoldenLines -> ERRO bad thing",
			"[app] writeGoldenLines -> ERRO from slog",
		},
	}, {
		format: "json",
		want: []string{
			`{"level":"info","ts":0,"name":"` + name + `","caller":"logging/output_test.go:30","msg":"hello","int":1,"str":"x","err":"context canceled","stringer":"S(v)","dur":1,"bytes":"aGk="}`,
			`{"level":"info","ts":0,"name":"` + name + `.sub","caller":"logging/output_test.go:32","msg":"with 3","w":2}`,
			`{"level":"error","ts":0,"name":"` + name + `","caller":"logging/output_test.go:34","msg":"bad thing","stacktrace":"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging.writeGoldenLines"}`,
			`{"level":"error","ts":0,"name":"app","caller":"logging/output_test.go:35","msg":"from slog","stacktrace":"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging.writeGoldenLines"}`,
		},
	}} {
		t.Run(tc.format, func(t *testing.T) {
			var buf bytes.Buffer
			t.Cleanup(func() { require.NoError(t, Init(Config{Writer: os.Stderr})) })
			require.NoError(t, Init(Config{Format: tc.format, LogSpec: "info", Writer: &buf}))
			buf.Reset() // drop what Init itself logs

			writeGoldenLines()

			got := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
			for i := range got {
				got[i] = jsonStack.ReplaceAllString(jsonTS.ReplaceAllString(got[i], `"ts":0`), `$1"`)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
