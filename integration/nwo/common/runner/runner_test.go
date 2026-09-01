/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner_test

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
)

// TestStopBringsTheProcessDown pins the contract nwo.StopFSCNode relies on: a
// caller that has sent Stop can wait for the process to be gone by polling
// ExitCode, rather than sleeping for a guessed interval and hoping.
//
// ExitCode must therefore mean exactly one thing at each moment — -1 while the
// process is alive, and the real code once it is not — and must be safe to read
// from the caller's goroutine while the runner's own monitor writes it.
func TestStopBringsTheProcessDown(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	r := runner.New(runner.Config{
		Name:    "sleeper",
		Command: exec.Command("sleep", "60"),
	})
	g.Expect(r.ExitCode()).To(gomega.Equal(-1), "a runner that has not started must not look exited")

	process := ifrit.Invoke(r)
	g.Eventually(process.Ready(), 10*time.Second).Should(gomega.BeClosed())
	g.Consistently(r.ExitCode, 200*time.Millisecond).Should(gomega.Equal(-1),
		"a running process must not look exited")

	r.Stop()

	g.Eventually(r, 30*time.Second).Should(gexec.Exit(), "Stop must bring the process down")
	g.Expect(r.ExitCode()).To(gomega.Equal(128+int(syscall.SIGTERM)),
		"a process killed by a signal reports 128+signal")
}

// TestCloneStartsUnexited guards the trap in Clone: a Runner reports "still
// running" with a sentinel rather than a zero value, so a clone that forgot to
// set it would claim to have exited cleanly before it had ever run. nwo restarts
// a node by cloning its runner, so anything waiting on that clone's exit would
// return immediately.
func TestCloneStartsUnexited(t *testing.T) {
	t.Parallel()
	g := gomega.NewWithT(t)

	r := runner.New(runner.Config{
		Name:    "sleeper",
		Command: exec.Command("sleep", "60"),
	})

	g.Expect(r.Clone().ExitCode()).To(gomega.Equal(-1), "a fresh clone must not look exited")
}
