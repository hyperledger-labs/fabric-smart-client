/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package profile

import (
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptions(t *testing.T) {
	t.Parallel()
	t.Run("WithPath", func(t *testing.T) {
		t.Parallel()
		p := &Profile{}
		opt := WithPath("test-path")
		err := opt(p)
		require.NoError(t, err)
		require.Equal(t, "test-path", p.path)

		optEmpty := WithPath("")
		err = optEmpty(p)
		require.Error(t, err)
		require.Contains(t, err.Error(), "path is required")
	})

	t.Run("WithAll", func(t *testing.T) {
		t.Parallel()
		p := &Profile{}
		opt := WithAll()
		err := opt(p)
		require.NoError(t, err)
		require.True(t, p.cpu)
		require.True(t, p.memoryAllocs)
		require.True(t, p.memoryHeap)
		require.True(t, p.mutex)
		require.True(t, p.blocker)
	})
}

func TestNew(t *testing.T) {
	t.Parallel()
	p, err := New(WithPath("test-path"))
	require.NoError(t, err)
	require.Equal(t, "test-path", p.path)
	require.Equal(t, DefaultMemProfileRate, p.memProfileRate)
	require.True(t, p.cpu) // Default is true
}

//nolint:paralleltest
func TestLifecycle(t *testing.T) {
	// We run these sequentially because they interact with global runtime state

	t.Run("Start and Stop CPU and Memory", func(t *testing.T) { //nolint:paralleltest
		tempDir := t.TempDir()
		p, err := New(WithPath(tempDir), WithAll())
		require.NoError(t, err)

		err = p.Start()
		require.NoError(t, err)

		// Check if files are created
		require.FileExists(t, filepath.Join(tempDir, "cpu.pprof"))
		require.FileExists(t, filepath.Join(tempDir, "mem-heap.pprof"))
		require.FileExists(t, filepath.Join(tempDir, "mem-allocs.pprof"))
		require.FileExists(t, filepath.Join(tempDir, "mutex.pprof"))
		require.FileExists(t, filepath.Join(tempDir, "block.pprof"))

		p.Stop()
	})

	t.Run("Start Error - Path is a file", func(t *testing.T) { //nolint:paralleltest
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "a-file")
		err := os.WriteFile(filePath, []byte("hello"), 0o644)
		require.NoError(t, err)

		p := &Profile{
			path: filePath,
		}
		err = p.Start()
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to create profile directory")
	})

	t.Run("Unknown Memory Profile", func(t *testing.T) { //nolint:paralleltest
		tempDir := t.TempDir()
		p := &Profile{path: tempDir}
		err := p.startMemProfile("unknown-type")
		require.NoError(t, err)
		p.Stop() // Should log error instead of panicking
	})

	t.Run("Partial Profiling", func(t *testing.T) { //nolint:paralleltest
		tempDir := t.TempDir()
		p, err := New(WithPath(tempDir)) // Default is cpu=true, others=false
		require.NoError(t, err)
		p.cpu = false // Disable CPU for this test to hit branches where nothing happens

		err = p.Start()
		require.NoError(t, err)
		p.Stop()

		// Verify no files created except maybe the dir
		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		require.Empty(t, entries)
	})
}

func TestNew_OptionError(t *testing.T) {
	t.Parallel()
	p, err := New(WithPath(""))
	require.ErrorContains(t, err, "path is required")
	require.Nil(t, p)
}

// TestStart_CreateFailure plants a *directory* where each profile file would go,
// so os.Create fails. This covers the create-error return inside every
// start*Profile as well as its propagation out of Start.
func TestStart_CreateFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		file    string
		wantErr string
		enable  func(*Profile)
	}{
		{"cpu", "cpu.pprof", "failed to create cpu profile file", func(p *Profile) { p.cpu = true }},
		{"memoryHeap", "mem-heap.pprof", "failed to create memory profile file", func(p *Profile) { p.memoryHeap = true }},
		{"memoryAllocs", "mem-allocs.pprof", "failed to create memory profile file", func(p *Profile) { p.memoryAllocs = true }},
		{"mutex", "mutex.pprof", "failed to create mutex profile file", func(p *Profile) { p.mutex = true }},
		{"blocker", "block.pprof", "failed to create block profile file", func(p *Profile) { p.blocker = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, tc.file), 0o755))
			p := &Profile{path: dir}
			tc.enable(p)
			require.ErrorContains(t, p.Start(), tc.wantErr)
		})
	}
}

//nolint:paralleltest // manipulates the process-wide CPU profiler
func TestStartCPUProfile_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "other.pprof"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	require.NoError(t, pprof.StartCPUProfile(f))
	t.Cleanup(pprof.StopCPUProfile)

	p := &Profile{path: dir}
	require.ErrorContains(t, p.startCPUProfile(), "failed to start cpu profile")
}

// TestStopTwice re-runs the closers against the files they already closed, which
// exercises the Sync/Close/WriteTo error branches inside them. Those errors are
// only logged, so the assertion left is that Stop puts the process-wide memory
// profile rate back and stays idempotent.
//
//nolint:paralleltest // manipulates process-wide profiling state
func TestStopTwice(t *testing.T) {
	before := runtime.MemProfileRate
	p, err := New(WithPath(t.TempDir()), WithAll())
	require.NoError(t, err)
	require.NoError(t, p.Start())
	p.Stop()
	require.Equal(t, before, runtime.MemProfileRate, "Stop should restore MemProfileRate")
	p.Stop()
	require.Equal(t, before, runtime.MemProfileRate)
}

//nolint:paralleltest
func TestInternalHelpers(t *testing.T) {
	t.Run("appendCloser", func(t *testing.T) { //nolint:paralleltest
		p := &Profile{}
		called := false
		p.appendCloser(func() {
			called = true
		})
		require.Len(t, p.closers, 1)
		p.Stop()
		require.True(t, called)
	})

	t.Run("MemProfileRate restoration", func(t *testing.T) { //nolint:paralleltest
		oldRate := runtime.MemProfileRate
		tempDir := t.TempDir()
		p := &Profile{
			path:           tempDir,
			memoryHeap:     true,
			memProfileRate: 1234,
		}

		err := p.startMemProfile("heap")
		require.NoError(t, err)
		require.Equal(t, 1234, runtime.MemProfileRate)

		p.Stop()
		require.Equal(t, oldRate, runtime.MemProfileRate)
	})
}
