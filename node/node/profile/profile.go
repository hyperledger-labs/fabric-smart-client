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
		return errors.Wrap(err, "failed to create cpu profile file")
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		return errors.Wrap(err, "failed to start cpu profile")
	}
	p.appendCloser(func() {
		logger.Infof("Stopping CPU profile")
		pprof.StopCPUProfile()
		if err := f.Sync(); err != nil {
			logger.Errorf("failed to flush cpu profile data to disk: %s", err)
		}
		if err := f.Close(); err != nil {
			logger.Errorf("failed to close cpu profile file: %s", err)
		}
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
		return errors.Wrap(err, "failed to create memory profile file")
	}
	old := runtime.MemProfileRate
	runtime.MemProfileRate = p.memProfileRate
	p.appendCloser(func() {
		logger.Infof("Stopping memory profile")
		if err := pprof.Lookup(memProfileType).WriteTo(f, 0); err != nil {
			logger.Errorf("failed to write memory profile: %s", err)
		}
		if err := f.Sync(); err != nil {
			logger.Errorf("failed to flush memory profile data to disk: %s", err)
		}
		if err := f.Close(); err != nil {
			logger.Errorf("failed to close memory profile file: %s", err)
		}
		runtime.MemProfileRate = old
	})

	return nil
}

func (p *Profile) startMutexProfile() error {
	fn := filepath.Join(p.path, "mutex.pprof")
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create mutex profile file")
	}
	runtime.SetMutexProfileFraction(1)

	p.appendCloser(func() {
		logger.Infof("Stopping mutex profile")
		if mp := pprof.Lookup("mutex"); mp != nil {
			if err := mp.WriteTo(f, 0); err != nil {
				logger.Errorf("failed to write mutex profile: %s", err)
			}
			if err := f.Sync(); err != nil {
				logger.Errorf("failed to flush mutex profile data to disk: %s", err)
			}
		}
		if err := f.Close(); err != nil {
			logger.Errorf("failed to close mutex profile file: %s", err)
		}
		runtime.SetMutexProfileFraction(0)
	})
	return nil
}

func (p *Profile) startBlockProfile() error {
	fn := filepath.Join(p.path, "block.pprof")
	f, err := os.Create(fn)
	if err != nil {
		return errors.Wrap(err, "failed to create block profile file")
	}
	runtime.SetBlockProfileRate(1)

	p.appendCloser(func() {
		if err := pprof.Lookup("block").WriteTo(f, 0); err != nil {
			logger.Errorf("failed to write block profile: %s", err)
		}
		if err := f.Sync(); err != nil {
			logger.Errorf("failed to flush block profile data to disk: %s", err)
		}
		if err := f.Close(); err != nil {
			logger.Errorf("failed to close block profile file: %s", err)
		}
		runtime.SetBlockProfileRate(0)
	})
	return nil
}
