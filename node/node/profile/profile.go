/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("fsc.node.start")

const DefaultMemProfileRate = 409

func compileOptions(opts ...Option) (*Profile, error) {
	txOptions := &Profile{
		memProfileRate: DefaultMemProfileRate,
		cpu:            true,
		closers:        []func(){},
	}
	for _, opt := range opts {
		if err := opt(txOptions); err != nil {
			return nil, err
		}
	}
	return txOptions, nil
}

func WithPath(path string) Option {
	return func(p *Profile) error {
		if path == "" {
			return errors.New("path is required")
		}
		p.path = path
		return nil
	}
}

func WithAll() Option {
	return func(p *Profile) error {
		p.cpu = true
		p.memoryAllocs = true
		p.memoryHeap = true
		p.mutex = true
		p.blocker = true
		return nil
	}
}

type Option func(*Profile) error

type Profile struct {
	path           string
	cpu            bool
	memProfileRate int
	memoryAllocs   bool
	memoryHeap     bool
	mutex          bool
	blocker        bool

	closers []func()
}

func New(opts ...Option) (*Profile, error) {
	return compileOptions(opts...)
}

func (p *Profile) Start() error {
	logger.Infof("Starting profiling")

	if err := os.MkdirAll(p.path, 0755); err != nil {
		return errors.Wrapf(err, "failed to create profile directory: %s", p.path)
	}
	if p.cpu {
		logger.Infof("Profiling CPU")
		if err := p.startCPUProfile(); err != nil {
			return err
		}
	}

	if p.memoryHeap {
		logger.Infof("Profiling memory heap")
		if err := p.startMemProfile("heap"); err != nil {
			return err
		}
	}

	if p.memoryAllocs {
		logger.Infof("Profiling memory allocations")
		if err := p.startMemProfile("allocs"); err != nil {
			return err
		}
	}

	if p.mutex {
		logger.Infof("Profiling mutex contention")
		if err := p.startMutexProfile(); err != nil {
			return err
		}
	}

	if p.blocker {
		logger.Infof("Profiling blocking")
		if err := p.startBlockProfile(); err != nil {
			return err
		}
	}

	logger.Infof("Profiling finished")
	return nil
}

func (p *Profile) Stop() {
	for _, closer := range p.closers {
		closer()
	}
}

func (p *Profile) startCPUProfile() error {
	fn := filepath.Join(p.path, "cpu.pprof")
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create cpu profile")
	}
	pprof.StartCPUProfile(f)
	p.appendCloser(func() {
		logger.Infof("Stopping CPU profile")
		pprof.StopCPUProfile()
		f.Close()
	})
	return nil

}

func (p *Profile) appendCloser(f func()) {
	p.closers = append(p.closers, f)
}

func (p *Profile) startMemProfile(memProfileType string) error {
	fn := filepath.Join(p.path, fmt.Sprintf("mem-%s.pprof", memProfileType))
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create memory profile")
	}
	old := runtime.MemProfileRate
	runtime.MemProfileRate = p.memProfileRate
	p.appendCloser(func() {
		logger.Infof("Stopping memory profile")
		pprof.Lookup(memProfileType).WriteTo(f, 0)
		f.Close()
		runtime.MemProfileRate = old
	})

	return nil
}

func (p *Profile) startMutexProfile() error {
	fn := filepath.Join(p.path, "mutex.pprof")
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create mutex profile")
	}
	runtime.SetMutexProfileFraction(1)

	p.appendCloser(func() {
		logger.Infof("Stopping mutex profile")
		if mp := pprof.Lookup("mutex"); mp != nil {
			mp.WriteTo(f, 0)
		}
		f.Close()
		runtime.SetMutexProfileFraction(0)
	})
	return nil
}

func (p *Profile) startBlockProfile() error {
	fn := filepath.Join(p.path, "block.pprof")
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create block profile")
	}
	runtime.SetBlockProfileRate(1)

	p.appendCloser(func() {
		pprof.Lookup("block").WriteTo(f, 0)
		f.Close()
		runtime.SetBlockProfileRate(0)
	})
	return nil
}
