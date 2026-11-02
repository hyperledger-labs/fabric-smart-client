/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package start

import (
	"context"
	stderrors "errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hyperledger-labs/fabric-smart-client/node/start/profile"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

const (
	nodeFuncName = "node"
	nodeCmdDes   = "Operate a fabric smart client node: start, migrate."
)

type Node interface {
	ID() string
	Start() error
	Stop()
	Callback() chan<- error
}

var logger = logging.MustGetLogger()

// Cmd returns the cobra command for Node
func Cmd(n Node) *cobra.Command {
	nodeCmd := &cobra.Command{
		Use:   nodeFuncName,
		Short: fmt.Sprint(nodeCmdDes),
		Long:  fmt.Sprint(nodeCmdDes),
	}
	nodeCmd.AddCommand(startCmd(n))
	if m, ok := n.(Migrator); ok {
		nodeCmd.AddCommand(migrateCmd(m))
	}
	return nodeCmd
}

// Migrator prepares the schema of every store a node opens, without starting
// the node.
type Migrator interface {
	Migrate(ctx context.Context, checkOnly bool) error
}

// Exit codes of `node migrate`.
const (
	// ExitFailed means the command could not run or a store could not be
	// checked or migrated: configuration, connection, lock or step failure.
	ExitFailed = 1
	// ExitUsage means invalid arguments; no store was opened.
	ExitUsage = 2
	// ExitPending means --check found steps pending.
	ExitPending = 10
	// ExitSchemaTooNew means a store was migrated by a newer binary.
	ExitSchemaTooNew = 11
)

// ExitError is an error that sets the process exit code; node.Execute exits
// with Code.
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

// exitCode returns the exit code of a Migrate error. A joined error, such as
// one per failing store, gives ExitFailed if any part is neither pending nor too
// new, since the check is then incomplete; otherwise ExitSchemaTooNew if any
// part is too new; otherwise ExitPending.
func exitCode(err error) int {
	tooNew, failed := false, false
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
	default:
		return ExitPending
	}
}

func usageError(_ *cobra.Command, err error) error { return &ExitError{Code: ExitUsage, Err: err} }

func migrateCmd(m Migrator) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Runs the pending schema steps of every store and exits.",
		Long: `Opens every store the node uses, without starting servers, P2P or delivery, and runs
their pending schema steps. With --check it runs no step and lists every pending step.
The command reads the node's full configuration, including its identities.

Exit codes: 0 every store is current (or has been migrated), 10 steps are pending (--check),
11 a store's schema is newer than this binary, 1 the command failed, 2 invalid arguments.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return usageError(cmd, errors.Errorf("unexpected arguments %q", args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			// No signal handler: a migrator waiting for the lock must die on SIGTERM.
			if err := m.Migrate(cmd.Context(), check); err != nil {
				return &ExitError{Code: exitCode(err), Err: err}
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
	return cmd
}

func startCmd(n Node) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Starts the fabric smart client node.",
		Long:  `Starts the fabric smart client node that interacts with the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return errors.New("trailing args detected")
			}
			cmd.SilenceUsage = true
			return serve(n)
		},
	}
}

func serve(n Node) error {
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
			logger.Errorf("error creating profiler: [%s]", err)
			n.Callback() <- err
			return err
		}
		// start profiler
		if err := profiler.Start(); err != nil {
			logger.Errorf("error starting profiler: [%s]", err)
			n.Callback() <- err
			return err
		}

		defer profiler.Stop()
	}

	// Not signal.NotifyContext: it reports only the first signal, and a second one must reach
	// n.Stop to skip the rest of the drain.
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	if err := n.Start(); err != nil {
		logger.Errorf("Failed starting platform [%s]", err)
		n.Callback() <- err
		return err
	}

	n.Callback() <- nil

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
