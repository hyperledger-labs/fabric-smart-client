/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

/*
Notice: This code is heavily inspired by the great work of ifrit and gomega.
- Ifrit (https://github.com/tedsuo/ifrit/blob/master/ginkgomon/ginkgomon.go)
was published under MIT License (MIT) with Copyright (c) 2014 Theodore Young.
- Gomega (https://github.com/onsi/gomega/blob/master/gexec/session.go)
was published under MIT License (MIT) with Copyright (c) 2013-2014 Onsi Fakhouri.
*/

package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/pkg/errors"
)

var logger = logging.MustGetLogger("nwo.runner")

// Config defines a Runner.
type Config struct {
	Command           *exec.Cmd     // process to be executed
	Name              string        // prefixes all output lines
	AnsiColorCode     string        // colors the output
	StartCheck        string        // text to match to indicate successful start.
	StartCheckTimeout time.Duration // how long to wait to see StartCheck
	Cleanup           func()        // invoked once the process exits
	Stdout, Stderr    io.Writer
}

type Runner struct {
	config            Config
	Command           *exec.Cmd
	Name              string
	AnsiColorCode     string
	StartCheck        string
	StartCheckTimeout time.Duration
	Cleanup           func()
	stop              chan os.Signal
	exitCode          int
}

// New creates a Runner from a config object. Runners must be created
// with New to properly initialize their internal state.
func New(config Config) *Runner {
	return &Runner{
		config:            config,
		Name:              config.Name,
		Command:           config.Command,
		AnsiColorCode:     config.AnsiColorCode,
		StartCheck:        config.StartCheck,
		StartCheckTimeout: config.StartCheckTimeout,
		Cleanup:           config.Cleanup,
		stop:              make(chan os.Signal),
		exitCode:          -1,
	}
}

func (r *Runner) Run(sigChan <-chan os.Signal, ready chan<- struct{}) error {
	var detectStartCheck chan bool
	allOutput := gbytes.NewBuffer()
	// silence buffer allows closing allOutput buffer once the process
	// has reached "ready" condition without throwing errors.
	out := NewSilenceBuffer(allOutput)

	var outWriter, errWriter io.Writer
	if r.config.Stdout != nil || r.config.Stderr != nil {
		logger.Infof("running [%s] with provided stdout/stderr", r.Name)
		outWriter = io.MultiWriter(out, ginkgo.GinkgoWriter, r.config.Stdout)
		errWriter = io.MultiWriter(out, ginkgo.GinkgoWriter, r.config.Stderr)
	} else {
		logger.Infof("running [%s] with ginkgo stdout/stderr", r.Name)
		outWriter = io.MultiWriter(out, ginkgo.GinkgoWriter)
		errWriter = io.MultiWriter(out, ginkgo.GinkgoWriter)
	}

	outWriter = gexec.NewPrefixedWriter(
		fmt.Sprintf("\x1b[32m[o]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
		outWriter,
	)

	errWriter = gexec.NewPrefixedWriter(
		fmt.Sprintf("\x1b[91m[e]\x1b[%s[%s]\x1b[0m ", r.AnsiColorCode, r.Name),
		errWriter,
	)

	r.Command.Stdout = outWriter
	r.Command.Stderr = errWriter

	exited := make(chan struct{})
	logger.Infof("start [%s] with args [%v]", r.Command.Path, r.Command.Args)
	err := r.Command.Start()
	if err != nil {
		return errors.Wrapf(err, "%s failed to start with err", r.Name)
	}
	logger.Infof("spawned [%s] (pid: %d) with args [%v]", r.Command.Path, r.Command.Process.Pid, r.Command.Args)

	go r.monitorForExit(exited)

	startCheckDuration := r.StartCheckTimeout
	if startCheckDuration == 0 {
		startCheckDuration = 5 * time.Second
	}

	var startCheckTimeout <-chan time.Time
	if r.StartCheck != "" {
		startCheckTimeout = time.After(startCheckDuration)
	}

	detectStartCheck = allOutput.Detect("%s", r.StartCheck)

	for {
		select {
		case <-detectStartCheck: // works even with empty string
			allOutput.CancelDetects()
			startCheckTimeout = nil
			detectStartCheck = nil
			// close our buffer that is used to detect ready state
			utils.IgnoreError(allOutput.Close())
			utils.IgnoreError(allOutput.Clear())
			close(ready)

		case <-startCheckTimeout:
			// clean up hanging process
			Expect(r.Command.Process.Signal(syscall.SIGKILL)).ToNot(HaveOccurred())
			EventuallyWithOffset(1, r).Should(gexec.Exit())

			// fail to start
			return errors.Errorf(
				"did not see %s in command's output within %s. full output:\n\n%s",
				r.StartCheck,
				startCheckDuration,
				string(allOutput.Contents()),
			)

		case signal := <-sigChan:
			if err := r.Command.Process.Signal(signal); err != nil {
				logger.Errorf("failed to send signal to process: %s", err)
			}

		case <-exited:
			if r.Cleanup != nil {
				r.Cleanup()
			}

			if r.exitCode == 0 {
				return nil
			}

			return errors.Errorf("exit status %d", r.exitCode)
		case signal := <-r.stop:
			logger.Infof("dispatch signal [%d] to process [%s]", signal, r.Name)
			if signal != nil {
				if err := r.Command.Process.Signal(signal); err != nil {
					logger.Errorf("failed to send signal [%d] to process [%s]", signal, r.Name)
				}
			}

		}
	}
}

func (r *Runner) monitorForExit(exited chan<- struct{}) {
	err := r.Command.Wait()
	status := r.Command.ProcessState.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		r.exitCode = 128 + int(status.Signal())
	} else {
		exitStatus := status.ExitStatus()
		if exitStatus == -1 && err != nil {
			r.exitCode = gexec.INVALID_EXIT_CODE
		}
		r.exitCode = exitStatus
	}

	close(exited)
}

func (r *Runner) Stop() {
	logger.Infof("Send SIGTERM to [%s]", r.Name)
	r.stop <- syscall.SIGTERM
}

func (r *Runner) PID() (string, int) {
	return r.Command.Path, r.Command.Process.Pid
}

func (r *Runner) Clone() *Runner {
	c := exec.Command(r.config.Command.Path)
	c.Args = r.config.Command.Args
	c.Env = r.config.Command.Env
	c.Dir = r.config.Command.Dir
	return &Runner{
		config:            r.config,
		Name:              r.config.Name,
		Command:           c,
		AnsiColorCode:     r.config.AnsiColorCode,
		StartCheck:        r.config.StartCheck,
		StartCheckTimeout: r.config.StartCheckTimeout,
		Cleanup:           r.config.Cleanup,
		stop:              make(chan os.Signal),
		exitCode:          -1,
	}
}

func (r *Runner) ExitCode() int {
	return r.exitCode
}
