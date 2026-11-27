/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// package cli (internal) to run the commands on a fake node.
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// Execute names the root command after the binary, so help, completion and version show
// that name.
func TestExecuteNamesRootAfterBinary(t *testing.T) { //nolint:paralleltest // sets os.Args and os.Stdout
	n := node.NewFromConfPath(t.TempDir())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"completion", "bash"}, "complete -o default -F __start_mynode mynode"},
		{[]string{"version"}, "mynode:\n"},
		{[]string{"node", "--help"}, "mynode node [command]"},
	} {
		out := captureStdout(t, func() {
			orig := os.Args
			defer func() { os.Args = orig }()
			os.Args = append([]string{"./bin/mynode"}, tc.args...)
			Execute(n, nil)
		})
		require.Contains(t, out, tc.want, "%v", tc.args)
	}
}

// captureStdout returns what f writes to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		raw, _ := io.ReadAll(r)
		done <- string(raw)
	}()
	f()
	os.Stdout = orig
	require.NoError(t, w.Close())
	return <-done
}

// The commands mounted under an application's own root run as under ours.
func TestCommandsUnderCustomRoot(t *testing.T) {
	t.Parallel()
	n := newFakeNode()
	root := &cobra.Command{Use: "app"}
	root.AddCommand(nodeCmd(n, nil), versionCmd())
	root.SetArgs([]string{"node", "migrate", "--check", "--contract"})
	require.NoError(t, root.Execute())
	require.Equal(t, []bool{true}, n.checkOnly)
	require.Len(t, n.opts[0], 1)

	n.migrateErr = errors.New("unreachable")
	root.SetArgs([]string{"node", "migrate"})
	root.SetErr(io.Discard)
	require.Equal(t, ExitFailed, ExitCode(root.Execute()))
	require.Len(t, Commands(node.NewFromConfPath(t.TempDir()), nil), 2)
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	require.Equal(t, 0, ExitCode(nil))
	require.Equal(t, ExitOlderOpener, ExitCode(fmt.Errorf("run: %w", &ExitError{Code: ExitOlderOpener})))
	require.Equal(t, ExitFailed, ExitCode(errors.New("boom")))
}

// Without a core.yaml, `version --json` runs, as release tooling runs it on a build machine;
// commands that need the configuration fail with the error that loading it gave.
func TestNoConfigFile(t *testing.T) {
	t.Parallel()
	n := node.NewFromConfPath(t.TempDir())
	root := &cobra.Command{Use: "app"}
	root.AddCommand(Commands(n, nil)...)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(io.Discard)

	root.SetArgs([]string{"version", "--json"})
	require.NoError(t, root.Execute())
	for _, args := range [][]string{{"node", "migrate"}, {"node", "peers", "--check"}, {"node", "start"}} {
		root.SetArgs(args)
		require.ErrorIs(t, root.Execute(), config.ErrNotFound, "%v", args)
	}
}
