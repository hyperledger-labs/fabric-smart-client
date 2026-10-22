/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"os/exec"
	"testing"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
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
