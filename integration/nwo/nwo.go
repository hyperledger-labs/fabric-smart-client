/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package nwo

import (
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/api"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fsc.integration")

type NWO struct {
	fscProcesss []ifrit.Process
	Processes   []ifrit.Process
	Members     grouper.Members

	Platforms              []api.Platform
	StartEventuallyTimeout time.Duration
	StopEventuallyTimeout  time.Duration
	ViewMembers            grouper.Members
}

func New(platforms ...api.Platform) *NWO {
	return &NWO{
		Platforms:              platforms,
		StartEventuallyTimeout: time.Minute,
		StopEventuallyTimeout:  time.Minute,
	}
}

func (n *NWO) KillFSC() {
	for _, process := range n.fscProcesss {
		process.Signal(syscall.SIGTERM)
		Eventually(process.Wait(), n.StopEventuallyTimeout).Should(Receive())
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
	logger.Infof("Load Configuration...")
	for _, platform := range n.Platforms {
		platform.Load()
	}
	logger.Infof("Load Configuration...done")
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

	logger.Infof("Run nodes...")

	// Execute members on their own stuff...
	Runner := grouper.NewOrdered(syscall.SIGTERM, members)
	process := ifrit.Invoke(Runner)
	n.Processes = append(n.Processes, process)
	Eventually(process.Ready(), n.StartEventuallyTimeout).Should(BeClosed())

	// Execute the fsc members in isolation so can be stopped and restarted as needed
	logger.Infof("Run FSC nodes...")
	for _, member := range fscMembers {
		logger.Infof("Run FSC node [%s]...", member)

		runner := runner.NewOrdered(syscall.SIGTERM, []grouper.Member{member})
		process := ifrit.Invoke(runner)
		Eventually(process.Ready(), n.StartEventuallyTimeout).Should(BeClosed())
		n.Processes = append(n.Processes, process)
		n.fscProcesss = append(n.fscProcesss, process)
	}

	logger.Infof("Post execution...")
	for _, platform := range n.Platforms {
		platform.PostRun()
	}
}

func (n *NWO) Stop() {
	logger.Infof("Stopping...")
	if len(n.Processes) != 0 {
		logger.Infof("Sending sigtem signal...")
		for _, process := range n.Processes {
			process.Signal(syscall.SIGTERM)
			Eventually(process.Wait(), n.StopEventuallyTimeout).Should(Receive())
		}
	}

	logger.Infof("Cleanup...")
	for _, platform := range n.Platforms {
		platform.Cleanup()
	}
	logger.Infof("Stopping...done!")
}

func (n *NWO) StopFSCNode(id string) {
	logger.Infof("Stopping fsc node [%s]...", id)
	for _, member := range n.ViewMembers {
		if strings.HasSuffix(member.Name, id) {
			member.Runner.(*runner.Runner).Stop()
			logger.Infof("Stopping fsc node [%s:%s] done", member.Name, id)
			return
		}
	}
	logger.Infof("Stopping fsc node [%s]...done", id)
}

func (n *NWO) StartFSCNode(id string) {
	logger.Infof("Starting fsc node [%s]...", id)
	for _, member := range n.ViewMembers {
		if strings.HasSuffix(member.Name, id) {
			runner := runner.NewOrdered(syscall.SIGTERM, []grouper.Member{{
				Name: id, Runner: member.Runner.(*runner.Runner).Clone(),
			}})
			member.Runner = runner
			process := ifrit.Invoke(runner)
			Eventually(process.Ready(), n.StartEventuallyTimeout).Should(BeClosed())
			n.Processes = append(n.Processes, process)
			logger.Infof("Starting fsc node [%s:%s] done", member.Name, id)
			return
		}
	}
	logger.Info("Starting fsc node [%s]...done", id)
}
