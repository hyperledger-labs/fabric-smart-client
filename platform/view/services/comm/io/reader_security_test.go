/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package io

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestVarintReader_OOMProtection(t *testing.T) {
	t.Parallel()
	// Create a buffer with a huge length prefix (e.g., 1GB)
	hugeLength := uint64(1024 * 1024 * 1024)
	buf := make([]byte, 10)
	n := binary.PutUvarint(buf, hugeLength)

	r := newVarintReader(bytes.NewReader(buf[:n]), testBufferSize, testMaxMessageSize)

	data, err := r.ReadData()
	require.Error(t, err)
	require.Nil(t, data)
	require.Contains(t, err.Error(), "exceeds max message size")
}

// A non-positive maxMessageSize must not mean "unlimited". Otherwise a peer can
// make the node allocate an arbitrary length with a 10-byte frame, since the
// claimed length is trusted for the allocation before any payload is read.
func TestVarintReader_NonPositiveMaxMessageSizeIsNotUnlimited(t *testing.T) {
	t.Parallel()

	// A 10-byte frame claiming 2^64-1 bytes, and no payload.
	frame := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}

	for _, maxMessageSize := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxMessageSize=%d", maxMessageSize), func(t *testing.T) {
			t.Parallel()
			r := newVarintReader(bytes.NewReader(frame), testBufferSize, maxMessageSize)

			data, err := r.ReadData()
			require.Error(t, err)
			require.Nil(t, data)
		})
	}
}

func TestVarintReader_NormalMessage(t *testing.T) {
	t.Parallel()
	msg := []byte("hello world")
	buf := make([]byte, 20)
	n := binary.PutUvarint(buf, uint64(len(msg)))
	copy(buf[n:], msg)

	r := newVarintReader(bytes.NewReader(buf[:n+len(msg)]), testBufferSize, testMaxMessageSize)

	data, err := r.ReadData()
	require.NoError(t, err)
	require.Equal(t, msg, data)
}

// readerFunc adapts a function to io.Reader.
type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// readFullGrowing must return exactly the requested bytes, across the initial
// buffer boundary and the geometric growth steps beyond it, and must not report
// success when the peer sends fewer bytes than it claimed.
func TestReadFullGrowing(t *testing.T) {
	t.Parallel()

	sizes := []int{
		0, 1, 1024,
		initialReadBufferCap - 1, initialReadBufferCap, initialReadBufferCap + 1,
		2 * initialReadBufferCap, 3*initialReadBufferCap + 7,
	}

	t.Run("reads exactly n bytes", func(t *testing.T) {
		t.Parallel()
		for _, n := range sizes {
			payload := make([]byte, n)
			for i := range payload {
				payload[i] = byte(i % 251)
			}

			got, err := readFullGrowing(bytes.NewReader(payload), n)
			require.NoError(t, err, "n=%d", n)
			require.Len(t, got, n, "n=%d", n)
			require.Equal(t, payload, got, "n=%d", n)
		}
	})

	t.Run("errors when the payload is short", func(t *testing.T) {
		t.Parallel()
		for _, n := range sizes {
			if n == 0 {
				continue
			}
			got, err := readFullGrowing(bytes.NewReader(make([]byte, n-1)), n)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF, "n=%d", n)
			require.Nil(t, got, "n=%d", n)
		}
	})

	t.Run("does not allocate the claimed length up front", func(t *testing.T) {
		t.Parallel()
		// A peer claims a huge length and sends one byte. The buffer's capacity is
		// what the allocation costs, so assert on that rather than on process-wide
		// MemStats, which other parallel tests pollute.
		huge := 512 * 1024 * 1024

		// The reader records the capacity of the slice it is handed, which is the
		// buffer readFullGrowing has allocated by the time it first reads.
		var seenCap int
		r := readerFunc(func(p []byte) (int, error) {
			seenCap = cap(p)
			return 0, io.EOF
		})

		_, err := readFullGrowing(r, huge)
		require.Error(t, err, "a short payload must not report success")
		require.LessOrEqual(t, seenCap, initialReadBufferCap,
			"allocated a %d-byte buffer for a claimed %d", seenCap, huge)
	})
}

// stubbornReader always returns (0, nil), which io.Reader explicitly permits.
// readFullGrowing must not spin on it: a peer that holds a stream open without
// sending data would otherwise burn a core in its per-stream goroutine.
type stubbornReader struct{ calls int }

func (s *stubbornReader) Read([]byte) (int, error) {
	s.calls++
	return 0, nil
}

func TestReadFullGrowing_doesNotSpinOnZeroByteReads(t *testing.T) {
	t.Parallel()

	sr := &stubbornReader{}
	done := make(chan struct{})
	var err error
	go func() {
		defer close(done)
		_, err = readFullGrowing(sr, 1024)
	}()

	select {
	case <-done:
		require.Error(t, err, "must give up rather than report success")
	case <-time.After(2 * time.Second):
		t.Fatalf("readFullGrowing spun on zero-byte reads (%d calls); it must give up", sr.calls)
	}
}

// The send limit must not be disabled by a non-positive value either. config.go
// clamps the configured value, but the writer keeps its own guard so the
// invariant holds for any future caller — matching varintReader.ReadData.
func TestProtoWriter_NonPositiveMaxSendMsgSizeIsNotUnlimited(t *testing.T) {
	t.Parallel()

	for _, maxSendMsgSize := range []int{0, -1} {
		t.Run(fmt.Sprintf("maxSendMsgSize=%d", maxSendMsgSize), func(t *testing.T) {
			t.Parallel()
			w := NewVarintProtoWriter(&bytes.Buffer{}, maxSendMsgSize)

			err := w.WriteMsg(&anypb.Any{TypeUrl: "test.type", Value: make([]byte, 1024)})
			require.Error(t, err)
			require.Contains(t, err.Error(), "must be positive")
		})
	}
}

// dataThenErrReader delivers every byte on its final Read while reporting a
// non-EOF error alongside them. bufio.Reader's large-read fast path
// (len(p) >= len(b.buf)) passes that combination straight through, and with
// streamReaderBufferSize at 4096 against a 10 MiB max message any message over
// 4 KiB takes it. A network stream reporting a reset with its last bytes looks
// exactly like this.
type dataThenErrReader struct {
	data []byte
	done bool
}

func (d *dataThenErrReader) Read(p []byte) (int, error) {
	if d.done {
		return 0, io.EOF
	}
	d.done = true
	return copy(p, d.data), errors.New("connection reset by peer")
}

// A complete message must be accepted even when the read that completed it also
// reported an error, matching io.ReadAtLeast. A short one must still fail.
func TestReadFullGrowing_completeMessageWithTrailingError(t *testing.T) {
	t.Parallel()

	// Larger than DefaultStreamReaderBufferSize so bufio takes the fast path.
	payload := make([]byte, 8192)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	t.Run("complete message is accepted", func(t *testing.T) {
		t.Parallel()
		r := bufio.NewReaderSize(&dataThenErrReader{data: payload}, 4096)

		got, err := readFullGrowing(r, len(payload))
		require.NoError(t, err)
		require.Equal(t, payload, got)
	})

	t.Run("short message still fails", func(t *testing.T) {
		t.Parallel()
		r := bufio.NewReaderSize(&dataThenErrReader{data: payload[:len(payload)-1]}, 4096)

		got, err := readFullGrowing(r, len(payload))
		require.Error(t, err)
		require.Nil(t, got)
	})
}
