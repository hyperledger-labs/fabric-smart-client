/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package cli

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/node/cli/profile"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a fabric smart client node: start, migrate, peers."
)

var logger = logging.MustGetLogger()

// nodeCmd returns the `node` command.
func nodeCmd(n fscNode, setup func() error) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   nodeFuncName,
		Short: fmt.Sprint(nodeCmdDes),
		Long:  fmt.Sprint(nodeCmdDes),
	}
	nodeCmd.AddCommand(startCmd(n, setup), migrateCmd(n), peersCmd(n))
	return nodeCmd
}

// Exit codes of `node migrate` and `node peers --check`.
const (
	// ExitFailed means the command could not run or a store could not be
	// checked or migrated: configuration, connection, lock or step failure.
	ExitFailed = 1
	// ExitUsage means invalid arguments; no store was opened.
	ExitUsage = 2
	// ExitPending means --check found steps pending, or a store requires a
	// contract step that has not run.
	ExitPending = 10
	// ExitSchemaTooNew means a store was migrated by a newer binary.
	ExitSchemaTooNew = 11
	// ExitOlderOpener means --contract found a store opened by a binary that
	// lacks a pending contract step.
	ExitOlderOpener = 12
	// ExitIncompatible means `node peers --check` found a peer this binary cannot talk to.
	ExitIncompatible = 11
)

// ExitError is an error that sets the process exit code; Execute exits with
// Code, see ExitCode.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return "exit code " + strconv.Itoa(e.Code)
	}
	return e.Err.Error()
}
func (e *ExitError) Unwrap() error { return e.Err }

// migrateExitCode returns the exit code of a Migrate error. A joined error, such as
// one per failing store, gives ExitFailed if any part is neither pending, too
// new nor blocked by an older opener, since the check is then incomplete;
// otherwise ExitSchemaTooNew if any part is too new; otherwise ExitOlderOpener
// if any part is blocked; otherwise ExitPending.
func migrateExitCode(err error) int {
	tooNew, older, failed := false, false, false
	var walk func(error)
	walk = func(err error) {
		for e := err; e != nil; e = stderrors.Unwrap(e) {
			if m, ok := e.(interface{ Unwrap() []error }); ok {
				for _, c := range m.Unwrap() {
					walk(c)
				}
				return
			}
		}
		switch {
		case errors.Is(err, migrate.ErrSchemaTooNew):
			tooNew = true
		case errors.Is(err, migrate.ErrOlderOpener):
			older = true
		case !errors.Is(err, migrate.ErrPending):
			failed = true
		}
	}
	walk(err)
	switch {
	case failed:
		return ExitFailed
	case tooNew:
		return ExitSchemaTooNew
	case older:
		return ExitOlderOpener
	default:
		return ExitPending
	}
}

func usageError(_ *cobra.Command, err error) error { return &ExitError{Code: ExitUsage, Err: err} }

func migrateCmd(m fscNode) *cobra.Command {
	var (
		check, contract bool
		forget          []string
	)
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Runs the pending schema steps of every store and exits.",
		Long: `Opens every store the node uses, without starting servers, P2P or delivery, and runs
their pending schema steps up to the first contract step. With --contract it also runs
contract steps, which binaries without them cannot read, once no such binary opens the store.
With --check it runs no step and lists every pending step.
The command reads the node's full configuration, including its identities.

Exit codes: 0 every store is current (or has been migrated), 10 steps are pending (--check)
or a store requires a contract step that has not run, 11 a store's schema is newer than
this binary, 12 a binary without a pending contract step opens a store (--contract),
1 the command failed, 2 invalid arguments.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageError(cmd, errors.Errorf("unexpected arguments %q", args))
			}
			if check && len(forget) > 0 {
				return usageError(cmd, errors.New("--forget-opener changes the database; it cannot be combined with --check"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			var opts []migrate.Option
			if contract {
				opts = append(opts, migrate.WithContract())
			}
			if len(forget) > 0 {
				opts = append(opts, migrate.WithForgetOpeners(forget...))
			}
			// No signal handler: a migrator waiting for the lock must die on SIGTERM.
			if err := m.Migrate(cmd.Context(), check, opts...); err != nil {
				return &ExitError{Code: migrateExitCode(err), Err: err}
			}
			if check {
				logger.Infof("no schema steps pending")
			} else {
				logger.Infof("schema migration done")
			}
			return nil
		},
	}
	cmd.SetFlagErrorFunc(usageError)
	cmd.Flags().BoolVar(&check, "check", false, "run no step; list pending steps and exit 10 if there are any")
	cmd.Flags().BoolVar(&contract, "contract", false, "also run contract steps; with --check, count pending contract steps as pending")
	cmd.Flags().StringArrayVar(&forget, "forget-opener", nil, "delete the opener rows of this FSC node id before migrating, for a node none of whose replicas runs a binary without the contract step anymore; repeatable")
	return cmd
}

func startCmd(n fscNode, setup func() error) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Starts the fabric smart client node.",
		Long:  `Starts the fabric smart client node that interacts with the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			return serve(cmd.Context(), n, setup)
		},
	}
}

// serve installs the node, runs setup, starts the node and stops it on SIGINT or SIGTERM; a
// second signal skips the rest of the drain. It stops the node if a step fails.
func serve(ctx context.Context, n fscNode, setup func() error) error {
	// config path
	configPath := os.Getenv("FSCNODE_CFG_PATH")
	if configPath == "" {
		configPath = "./"
	}

	// profile
	enableProfile := false
	enableProfileStr := os.Getenv("FSCNODE_PROFILER")
	if len(enableProfileStr) != 0 {
		var err error
		enableProfile, err = strconv.ParseBool(enableProfileStr)
		if err != nil {
			logger.Infof("Error parsing boolean environment variable FSCNODE_PROFILER: %s\n", err.Error())
		}
	}

	if enableProfile {
		logger.Infof("Profiling enabled")
		profiler, err := profile.New(profile.WithPath(configPath), profile.WithAll())
		if err != nil {
			// Cobra's error printing is silenced; this line is the node's report of the
			// failure.
			logger.Errorf("error creating profiler: [%s]", err)
			return err
		}
		// start profiler
		if err := profiler.Start(); err != nil {
			// Cobra's error printing is silenced; this line is the node's report of the
			// failure.
			logger.Errorf("error starting profiler: [%s]", err)
			return err
		}

		defer profiler.Stop()
	}

	// Not signal.NotifyContext: it reports only the first signal, and a second one must reach
	// n.Stop to skip the rest of the drain.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	if err := start(ctx, n, setup); err != nil {
		// Cobra's error printing is silenced; this line is the node's report of the
		// failure.
		logger.Errorf("Failed starting platform [%s]", err)
		n.Stop()
		return err
	}

	logger.Infof("Started peer with ID=[%s]", n.ID())

	<-signals
	logger.Infof("Received signal, exiting...")
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-signals:
			logger.Infof("Received second signal, skipping the rest of the drain...")
			n.Stop()
		case <-stopped:
		}
	}()
	n.Stop()

	return nil
}

func start(ctx context.Context, n fscNode, setup func() error) error {
	if err := n.Install(); err != nil {
		return err
	}
	if setup != nil {
		if err := setup(); err != nil {
			return err
		}
	}
	return n.Start(ctx)
}
