/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/context"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/monitoring"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/onsi/gomega"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
)

var logger = logging.MustGetLogger("fsc.integration")

type Process interface {
	PID() (string, int)
}

type Group interface {
	Members() grouper.Members
}

type NWO struct {
	FSCProcesses      []ifrit.Process
	Processes         []ifrit.Process
	TerminationSignal os.Signal
	Members           grouper.Members

	Platforms              []api.Platform
	StartEventuallyTimeout time.Duration
	StopEventuallyTimeout  time.Duration
	ViewMembers            grouper.Members

	ctx       *context.Context
	isLoading bool
}

func New(ctx *context.Context, platforms ...api.Platform) *NWO {
	return &NWO{
		ctx:                    ctx,
		Platforms:              platforms,
		StartEventuallyTimeout: time.Minute,
		StopEventuallyTimeout:  time.Minute,
		TerminationSignal:      syscall.SIGTERM,
	}
}

func (n *NWO) KillFSC() {
	for _, process := range n.FSCProcesses {
		process.Signal(syscall.SIGTERM)
		gomega.Eventually(process.Wait(), n.StopEventuallyTimeout).Should(gomega.Receive())
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
	f, err := os.Create(filepath.Join(n.ctx.RootDir(), "pids.txt"))
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

	// Execute members on their own stuff...
	Runner := grouper.NewOrdered(n.TerminationSignal, members)
	process := ifrit.Invoke(Runner)
	n.Processes = append(n.Processes, process)
	gomega.Eventually(process.Ready(), n.StartEventuallyTimeout).Should(gomega.BeClosed())
	n.storePIDs(f, members)

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

	// Execute the fsc members in isolation so can be stopped and restarted as needed
	logger.Infof("Run FSC nodes...")
	for _, member := range fscMembers {
		logger.Infof("Run FSC node [%s]...", member.Name)

		runner := grouper.NewOrdered(n.TerminationSignal, []grouper.Member{member})
		process := ifrit.Invoke(runner)
		gomega.Eventually(process.Ready(), n.StartEventuallyTimeout).Should(gomega.BeClosed())
		n.Processes = append(n.Processes, process)
		n.FSCProcesses = append(n.FSCProcesses, process)
	}
	n.storePIDs(f, fscMembers)

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
		logger.Infof("Sending sigterm signal...")
		for _, process := range n.Processes {
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

func (n *NWO) StopFSCNode(id string) {
	logger.Infof("Search FSC node [%s]...", id)
	for _, member := range n.ViewMembers {
		if strings.HasPrefix(member.Name, fmt.Sprintf("fsc.%s.", id)) {
			logger.Infof("FSC node [%s] found. Stopping...", id)
			member.Runner.(*runner.Runner).Stop()
			logger.Infof("FSC node [%s:%s] stopped", member.Name, id)
			return
		}
	}
	logger.Infof("FSC node [%s] not found", id)
}

func (n *NWO) StartFSCNode(id string) {
	logger.Infof("Search FSC node [%s]...", id)
	for i, member := range n.ViewMembers {
		if strings.HasPrefix(member.Name, fmt.Sprintf("fsc.%s.", id)) {
			logger.Infof("FSC node [%s] found. Starting...", id)
			member.Runner = member.Runner.(*runner.Runner).Clone()
			n.ViewMembers[i] = member
			process := ifrit.Invoke(member.Runner)
			gomega.Eventually(process.Ready(), n.StartEventuallyTimeout).Should(gomega.BeClosed())
			n.Processes = append(n.Processes, process)
			logger.Infof("FSC node [%s:%s] started", member.Name, id)
			return
		}
	}
	logger.Infof("FSC node [%s] not found", id)
}

func (n *NWO) storePIDs(f *os.File, members grouper.Members) {
	for _, member := range members {
		switch r := member.Runner.(type) {
		case Process:
			path, pid := r.PID()
			_, err := fmt.Fprintf(f, "%s %d\n", path, pid)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		case Group:
			n.storePIDs(f, r.Members())
		}
	}
}

func (n *NWO) PrometheusAPI() (v1.API, error) {
	for _, platform := range n.Platforms {
		if metricsPlatform, ok := platform.(*monitoring.Platform); ok {
			return metricsPlatform.PrometheusAPI(), nil
		}
	}
	return nil, fmt.Errorf("no Prometheus API available on any platform")
}
