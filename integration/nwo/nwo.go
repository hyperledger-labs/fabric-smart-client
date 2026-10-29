/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/integration/reporting/jaeger"
	"github.com/hyperledger-labs/fabric-smart-client/integration/reporting/prometheus"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

var logger = logging.MustGetLogger()

type Process interface {
	PID() (string, int)
}

// NWO runs the members of its platforms. Every member runs as its own ifrit
// process, so a test can stop, kill, or restart one FSC node, Fabric peer, or
// Fabric orderer while the rest of the network keeps running.
type NWO struct {
	Processes []ifrit.Process
	Members   grouper.Members

	Platforms              []api.Platform
	StartEventuallyTimeout time.Duration
	StopEventuallyTimeout  time.Duration
	ViewMembers            grouper.Members

	nwoCtx    *context.NWOContext
	isLoading bool
}

func New(nwoCtx *context.NWOContext, platforms ...api.Platform) *NWO {
	return &NWO{
		nwoCtx:                 nwoCtx,
		Platforms:              platforms,
		StartEventuallyTimeout: time.Minute,
		StopEventuallyTimeout:  time.Minute,
	}
}

func (n *NWO) Generate() {
	logger.Infof("Generate Configuration...")
	for _, platform := range n.Platforms {
		platform.GenerateConfigTree()
	}

	for _, platform := range n.Platforms {
		platform.GenerateArtifacts()
	}
	logger.Infof("Generate Configuration...done!")
}

func (n *NWO) Load() {
	n.isLoading = true
	logger.Infof("Load Configuration...")
	for _, platform := range n.Platforms {
		platform.Load()
	}
	logger.Infof("Load Configuration...done")
}

type preRun interface {
	PreRun()
}

func (n *NWO) Start() {
	logger.Infof("Starting...")

	logger.Infof("Collect members...")
	members := grouper.Members{}

	fscMembers := grouper.Members{}
	for _, platform := range n.Platforms {
		logger.Infof("From [%s]...", platform.Type())
		m := platform.Members()
		if m == nil {
			continue
		}
		for _, member := range m {
			logger.Infof("Adding member [%s]", member.Name)
		}

		if platform.Type() == "fsc" {
			fscMembers = append(fscMembers, m...)
		} else {
			members = append(members, m...)
		}

	}
	n.Members = members
	n.ViewMembers = fscMembers

	// store PIDs of all processes
	f, err := os.Create(filepath.Join(n.nwoCtx.RootDir(), "pids.txt"))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		// write PIDs to file
		gomega.Expect(f.Sync()).NotTo(gomega.HaveOccurred())
		gomega.Expect(f.Close()).NotTo(gomega.HaveOccurred())
	}()

	logger.Infof("Run platform nodes...")

	for _, platform := range n.Platforms {
		if p, ok := platform.(preRun); ok && platform.Type() != "fsc" {
			p.PreRun()
		}
	}

	// Start the members one by one and in order, so orderers are up before peers.
	for _, member := range members {
		n.invoke(member)
	}
	storePIDs(f, members)

	logger.Infof("Post execution for nodes...")
	for _, platform := range n.Platforms {
		if platform.Type() != "fsc" {
			platform.PostRun(n.isLoading)
		}
	}

	// deal with fsc nodes separately
	if len(fscMembers) == 0 {
		logger.Infof("Skipping starting FSC nodes and post execution as no FSC members are defined")
		return
	}

	for _, platform := range n.Platforms {
		if p, ok := platform.(preRun); ok && platform.Type() == "fsc" {
			p.PreRun()
		}
	}

	logger.Infof("Run FSC nodes...")
	for _, member := range fscMembers {
		n.invoke(member)
	}
	storePIDs(f, fscMembers)

	logger.Infof("Post execution for FSC nodes...")
	for _, platform := range n.Platforms {
		if platform.Type() == "fsc" {
			platform.PostRun(n.isLoading)
		}
	}
}

func (n *NWO) Stop() {
	logger.Infof("Stopping...")
	if len(n.Processes) != 0 {
		// Stop in reverse start order, so FSC nodes go before the network they
		// talk to; a member restarted by a test is stopped first.
		logger.Infof("Sending sigterm signal...")
		for _, process := range slices.Backward(n.Processes) {
			process.Signal(syscall.SIGTERM)
			gomega.Eventually(process.Wait(), n.StopEventuallyTimeout).Should(gomega.Receive())
		}
	}

	logger.Infof("Cleanup...")
	for _, platform := range n.Platforms {
		platform.Cleanup()
	}
	logger.Infof("Stopping...done!")
}

// StopFSCNode stops the named FSC node and returns once its process is gone.
//
// Waiting matters because the node holds resources a replacement cannot share:
// its listening ports and its vault. A caller that restarts the node right away
// would otherwise race the old process for them. StartFSCNode already waits for
// readiness; this is its counterpart.
func (n *NWO) StopFSCNode(id string) {
	if i := n.fscIndex(id); i >= 0 {
		r := n.ViewMembers[i].Runner.(*runner.Runner)
		n.awaitExit(r, r.Stop)
	}
}

// KillFSCNode sends SIGKILL to the named FSC node and returns once its process
// is gone. Unlike StopFSCNode the node runs none of its shutdown logic, which
// makes this the primitive for crash-recovery tests. Restart the node with
// StartFSCNode.
func (n *NWO) KillFSCNode(id string) {
	if i := n.fscIndex(id); i >= 0 {
		r := n.ViewMembers[i].Runner.(*runner.Runner)
		n.awaitExit(r, r.Kill)
	}
}

// PauseFSCNode freezes the named FSC node with SIGSTOP. Its process and
// connections stay, so a replica falls behind its peers without restarting;
// ResumeFSCNode continues it.
func (n *NWO) PauseFSCNode(id string) {
	if i := n.fscIndex(id); i >= 0 {
		n.ViewMembers[i].Runner.(*runner.Runner).Pause()
	}
}

// ResumeFSCNode continues the named FSC node frozen by PauseFSCNode.
func (n *NWO) ResumeFSCNode(id string) {
	if i := n.fscIndex(id); i >= 0 {
		n.ViewMembers[i].Runner.(*runner.Runner).Resume()
	}
}

// StartFSCNode starts the named FSC node, which must have been stopped or
// killed before, and returns once it is ready.
func (n *NWO) StartFSCNode(id string) {
	if i := n.fscIndex(id); i >= 0 {
		n.ViewMembers[i] = n.restart(n.ViewMembers[i])
	}
}

// StopMember stops the platform member with the given name, such as a Fabric
// peer or orderer, and returns once its process is gone. The test fails if no
// such member exists.
func (n *NWO) StopMember(name string) {
	r := n.Members[n.memberIndex(name)].Runner.(*runner.Runner)
	n.awaitExit(r, r.Stop)
}

// StartMember starts the platform member with the given name, which must have
// been stopped before, and returns once it is ready. The test fails if no such
// member exists.
func (n *NWO) StartMember(name string) {
	i := n.memberIndex(name)
	n.Members[i] = n.restart(n.Members[i])
}

func (n *NWO) memberIndex(name string) int {
	i := slices.IndexFunc(n.Members, func(m grouper.Member) bool { return m.Name == name })
	gomega.Expect(i).NotTo(gomega.Equal(-1), "member [%s] not found", name)
	return i
}

// fscIndex returns the index in ViewMembers of the FSC node with the given id,
// or -1 if there is none. id is a node name, whose first replica is returned, or
// a replica name such as bob.1.
func (n *NWO) fscIndex(id string) int {
	prefix := fmt.Sprintf("fsc.%s.", id)
	i := slices.IndexFunc(n.ViewMembers, func(m grouper.Member) bool {
		return m.Name == "fsc."+id || strings.HasPrefix(m.Name, prefix)
	})
	if i < 0 {
		logger.Infof("FSC node [%s] not found", id)
	}
	return i
}

// awaitExit sends the signal through send and waits for r's process to exit.
func (n *NWO) awaitExit(r *runner.Runner, send func()) {
	send()
	gomega.Eventually(r, n.StopEventuallyTimeout).Should(gexec.Exit(), "[%s] did not exit", r.Name)
	logger.Infof("[%s] stopped", r.Name)
}

// restart runs a fresh copy of the stopped member's runner and returns the
// member holding it; a Runner can run only once.
func (n *NWO) restart(member grouper.Member) grouper.Member {
	member.Runner = member.Runner.(*runner.Runner).Clone()
	n.invoke(member)
	return member
}

// invoke runs member as its own process and waits until it is ready. The
// process is recorded before the wait, so Stop also ends one that never
// became ready.
func (n *NWO) invoke(member grouper.Member) {
	logger.Infof("Run [%s]...", member.Name)
	process := ifrit.Background(member.Runner)
	n.Processes = append(n.Processes, process)
	gomega.Eventually(process.Ready(), n.StartEventuallyTimeout).Should(gomega.BeClosed(), "[%s] did not start", member.Name)
}

func storePIDs(f *os.File, members grouper.Members) {
	for _, member := range members {
		if r, ok := member.Runner.(Process); ok {
			path, pid := r.PID()
			_, err := fmt.Fprintf(f, "%s %d\n", path, pid)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}
}

func (n *NWO) PrometheusReporter() (prometheus.Reporter, error) {
	for _, platform := range n.Platforms {
		if metricsPlatform, ok := platform.(*monitoring.Platform); ok {
			return metricsPlatform.PrometheusReporter(), nil
		}
	}
	return nil, errors.New("no Prometheus API available on any platform")
}

func (n *NWO) JaegerReporter() (jaeger.Reporter, error) {
	for _, platform := range n.Platforms {
		if metricsPlatform, ok := platform.(*monitoring.Platform); ok {
			return metricsPlatform.JaegerReporter(), nil
		}
	}
	return nil, errors.New("no Jaeger API available on any platform")
}
