/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/stretchr/testify/require"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
)

// TestStopMemberLeavesSiblingsRunning guards the reason every member runs as its
// own process: stopping one peer or orderer must not take the others down, as
// it would inside a shared ifrit group.
func TestStopMemberLeavesSiblingsRunning(t *testing.T) { //nolint:paralleltest // NWO asserts through the global gomega
	gomega.RegisterTestingT(t)
	g := gomega.NewWithT(t)

	sleeper := func(name string) grouper.Member {
		return grouper.Member{Name: name, Runner: runner.New(runner.Config{Name: name, Command: exec.Command("sleep", "60")})}
	}
	n := &NWO{StartEventuallyTimeout: 10 * time.Second, StopEventuallyTimeout: 10 * time.Second}
	n.Members = grouper.Members{sleeper("fabric.default.peer.peer0"), sleeper("fabric.default.peer.peer1")}
	for _, m := range n.Members {
		n.invoke(m)
	}
	t.Cleanup(func() {
		for _, m := range n.Members {
			_ = m.Runner.(*runner.Runner).Command.Process.Kill()
		}
	})

	n.StopMember("fabric.default.peer.peer0")
	g.Consistently(n.Members[1].Runner, time.Second).ShouldNot(gexec.Exit(), "stopping peer0 took peer1 down")

	n.StopMember("fabric.default.peer.peer1")
	n.StopMember("fabric.default.peer.peer1") // must return, not block, on a process that is gone

	n.StartMember("fabric.default.peer.peer0")
	g.Expect(n.Members[0].Runner).NotTo(gexec.Exit(), "peer0 did not come back")
}

// recorder returns a member whose process writes its name to log when it is
// asked to stop, so the order of the lines is the order Stop signaled them in.
func recorder(name, log string) grouper.Member {
	script := `trap 'echo ` + name + ` >> ` + log + `; exit 0' TERM; echo ready; while :; do sleep 0.1; done`
	return grouper.Member{Name: name, Runner: runner.New(runner.Config{
		Name:       name,
		Command:    exec.Command("sh", "-c", script),
		StartCheck: "ready", // the trap is installed before this line is printed
	})}
}

// TestStopInReverseStartOrder pins the teardown order: FSC nodes, started
// last, must go before the orderers and peers they talk to.
func TestStopInReverseStartOrder(t *testing.T) { //nolint:paralleltest // NWO asserts through the global gomega
	gomega.RegisterTestingT(t)
	g := gomega.NewWithT(t)

	log := filepath.Join(t.TempDir(), "stopped")
	n := &NWO{StartEventuallyTimeout: 10 * time.Second, StopEventuallyTimeout: 10 * time.Second}
	for _, name := range []string{"orderer", "peer", "fsc"} {
		n.invoke(recorder(name, log))
	}

	n.Stop()

	stopped, err := os.ReadFile(log)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(strings.Fields(string(stopped))).To(gomega.Equal([]string{"fsc", "peer", "orderer"}))
}

// TestStopEndsAMemberThatNeverBecameReady guards teardown on the path where it
// matters most: a member that fails to start must still be ended by Stop, or it
// outlives the suite and keeps its ports.
func TestStopEndsAMemberThatNeverBecameReady(t *testing.T) { //nolint:paralleltest // NWO asserts through the global gomega
	gomega.RegisterTestingT(t)
	g := gomega.NewWithT(t)

	r := runner.New(runner.Config{
		Name:              "never-ready",
		Command:           exec.Command("sleep", "60"),
		StartCheck:        "a line sleep never prints",
		StartCheckTimeout: time.Minute,
	})
	n := &NWO{StartEventuallyTimeout: 200 * time.Millisecond, StopEventuallyTimeout: 10 * time.Second}
	t.Cleanup(func() { _ = r.Command.Process.Kill() })

	// Fail the way ginkgo does, by panicking out of the failed assertion, so
	// nothing after it in invoke runs.
	g.Expect(failsAndAborts(func() {
		n.invoke(grouper.Member{Name: "never-ready", Runner: r})
	})).To(gomega.BeTrue(), "invoke must fail when the member does not become ready")
	gomega.RegisterTestingT(t)

	n.Stop()

	g.Expect(r).To(gexec.Exit(), "Stop left the member that never became ready running")
}

// failsAndAborts runs f with a fail handler that panics, as ginkgo.Fail does,
// and reports whether f failed. The caller must register its fail handler
// again afterwards.
func failsAndAborts(f func()) (failed bool) {
	type failure struct{}
	gomega.RegisterFailHandler(func(string, ...int) { panic(failure{}) })
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(failure); !ok {
				panic(p)
			}
			failed = true
		}
	}()
	f()
	return false
}

func TestFscIndex(t *testing.T) {
	t.Parallel()
	n := &NWO{ViewMembers: grouper.Members{{Name: "fsc.bob.0"}, {Name: "fsc.bob.1"}, {Name: "fsc.bobby.0"}}}
	for id, want := range map[string]int{"bob": 0, "bob.0": 0, "bob.1": 1, "bobby": 2, "bo": -1, "bob.2": -1} {
		require.Equal(t, want, n.fscIndex(id), "fscIndex(%q)", id)
	}
}
