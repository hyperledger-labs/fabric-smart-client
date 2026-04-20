/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package profile

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestLifecycle(t *testing.T) {
	// nolint:paralleltest
	// We run these sequentially because they interact with global runtime state

	t.Run("Start and Stop CPU and Memory", func(t *testing.T) {
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

	t.Run("Start Error - Path is a file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "a-file")
		err := os.WriteFile(filePath, []byte("hello"), 0644)
		require.NoError(t, err)

		p := &Profile{
			path: filePath,
		}
		err = p.Start()
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to create profile directory")
	})

	t.Run("Unknown Memory Profile", func(t *testing.T) {
		tempDir := t.TempDir()
		p := &Profile{path: tempDir}
		err := p.startMemProfile("unknown-type")
		require.NoError(t, err)
		p.Stop() // Should log error instead of panicking
	})

	t.Run("Partial Profiling", func(t *testing.T) {
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

func TestInternalHelpers(t *testing.T) {
	// nolint:paralleltest
	t.Run("appendCloser", func(t *testing.T) {
		p := &Profile{}
		called := false
		p.appendCloser(func() {
			called = true
		})
		require.Len(t, p.closers, 1)
		p.Stop()
		require.True(t, called)
	})

	t.Run("MemProfileRate restoration", func(t *testing.T) {
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
