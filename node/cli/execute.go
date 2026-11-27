/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package cli holds the cobra commands of a Fabric Smart Client node binary:
//   - `node start` to install, set up and start the node and stop it on SIGINT or SIGTERM;
//   - `node migrate` to prepare the schema of every store and exit; `--check` only checks it;
//   - `node peers --check` to check that the binary can talk to the configured peers;
//   - `version` to print the version of the binary; `--json` adds ranges and store steps.
//
// Execute runs them under a root command named after the binary; Commands returns them to
// mount under an application's own root. The node itself, package node, does not need them.
package cli

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// fscNode is what the commands use of a *node.Node.
type fscNode interface {
	ID() string
	Install() error
	Start(ctx context.Context) error
	Stop()
	Migrate(ctx context.Context, checkOnly bool, opts ...migrate.Option) error
	Config() (driver.ConfigService, error)
}

// Execute runs the command line under a root command named after the binary and exits the
// process with ExitCode if a command fails. setup, which may be nil, runs only for `node
// start`, after n.Install and before n.Start, so views and services it registers exist before
// the node serves.
func Execute(n *node.Node, setup func() error) {
	root := &cobra.Command{Use: filepath.Base(os.Args[0])}
	root.AddCommand(Commands(n, setup)...)
	if err := root.Execute(); err != nil {
		os.Exit(ExitCode(err))
	}
}

// Commands returns the `node` and `version` commands, to add to an application's own root
// command; setup is as for Execute.
func Commands(n *node.Node, setup func() error) []*cobra.Command {
	return []*cobra.Command{nodeCmd(n, setup), versionCmd()}
}

// ExitCode returns the process exit code for the error of a command: 0 for nil, the Code of
// an ExitError in its chain, ExitFailed otherwise.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := stderrors.AsType[*ExitError](err); ok {
		return e.Code
	}
	return ExitFailed
}
