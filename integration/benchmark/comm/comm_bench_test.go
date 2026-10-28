/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/gologshim"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/benchmark"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// bye asks the echo responder to end the session. Benchmark payloads are all zeros, so they
// never collide with it.
var bye = []byte("bye")

const replyTimeout = 10 * time.Second

func TestMain(m *testing.M) {
	// go test interleaves log output with benchmark results, which breaks benchstat, and node
	// shutdown always logs errors: stream-close errors through FSC's logger, aborted TLS
	// handshakes through the standard logger, and libp2p listener errors through slog. Logs are
	// therefore off unless FSC_LOGSPEC asks for them; benchmark failures are reported through
	// testing.B regardless.
	spec := os.Getenv("FSC_LOGSPEC")
	if spec == "" {
		spec = "panic"
		log.SetOutput(io.Discard)
		gologshim.SetDefaultHandler(slog.DiscardHandler)
	}
	logging.Init(logging.Config{LogSpec: spec})
	os.Exit(m.Run())
}

// BenchmarkRoundTrip measures the steady-state latency of one request/response exchange on a
// single long-lived session, for several payload sizes. Session setup and the first dial are
// excluded.
func BenchmarkRoundTrip(b *testing.B) {
	for _, t := range transports {
		for _, size := range []int{64, 1 << 10, 64 << 10, 1 << 20} {
			b.Run(fmt.Sprintf("transport=%s/size=%s", t.name, byteSize(size)), func(b *testing.B) {
				a, z := t.pair(b)
				serveEcho(b, z)
				s := openSession(b, a, z)
				payload := make([]byte, size)
				lat := make([]time.Duration, b.N)

				b.SetBytes(int64(2 * size))
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					start := time.Now()
					if err := roundTrip(b.Context(), s, payload); err != nil {
						b.Fatal(err)
					}
					lat[i] = time.Since(start)
				}
				benchmark.ReportTPS(b)
				reportPercentiles(b, lat)
			})
		}
	}
}

// BenchmarkSessionSetup measures the cost of a full session lifecycle: open a session, exchange
// one message, and tear the session down on both sides. The transport connection between the
// two nodes is established before timing starts.
func BenchmarkSessionSetup(b *testing.B) {
	for _, t := range transports {
		b.Run("transport="+t.name, func(b *testing.B) {
			a, z := t.pair(b)
			serveEcho(b, z)
			closeSession(b.Context(), openSession(b, a, z))
			payload := make([]byte, 64)
			lat := make([]time.Duration, b.N)

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				start := time.Now()
				s, err := a.NewSession("", "", z.Address, []byte(z.ID))
				if err != nil {
					b.Fatal(err)
				}
				if err := roundTrip(b.Context(), s, payload); err != nil {
					b.Fatal(err)
				}
				closeSession(b.Context(), s)
				lat[i] = time.Since(start)
			}
			benchmark.ReportTPS(b)
			reportPercentiles(b, lat)
		})
	}
}

// BenchmarkParallelRoundTrip measures throughput when GOMAXPROCS senders exchange messages
// concurrently, each on its own session, over the same pair of nodes.
func BenchmarkParallelRoundTrip(b *testing.B) {
	for _, t := range transports {
		b.Run("transport="+t.name, func(b *testing.B) {
			a, z := t.pair(b)
			serveEcho(b, z)
			// RunParallel starts GOMAXPROCS goroutines, since parallelism is left at 1. One session
			// per goroutine is opened up front, so that dialing and session setup stay out of the
			// timed region. The sessions stay open until the nodes stop: closing one while others
			// are still exchanging messages loses replies on libp2p (see README).
			sessions := make(chan view.Session, runtime.GOMAXPROCS(0))
			for range cap(sessions) {
				sessions <- openSession(b, a, z)
			}
			payload := make([]byte, 1<<10)
			lat := make([]time.Duration, b.N)
			var next atomic.Int64

			b.SetBytes(int64(2 * len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				s := <-sessions
				for pb.Next() {
					start := time.Now()
					if err := roundTrip(b.Context(), s, payload); err != nil {
						b.Error(err)
						return
					}
					lat[next.Add(1)-1] = time.Since(start)
				}
			})
			benchmark.ReportTPS(b)
			reportPercentiles(b, lat[:next.Load()])
		})
	}
}

// BenchmarkMesh measures throughput and latency when every node of a full mesh exchanges
// messages with every other node at the same time, over a given number of long-lived sessions
// per ordered pair of nodes. Each op is one 1 KiB round trip on one of those sessions.
func BenchmarkMesh(b *testing.B) {
	for _, t := range transports {
		for _, n := range []int{3, 5} {
			for _, perPair := range []int{1, 4} {
				b.Run(fmt.Sprintf("transport=%s/nodes=%d/sessions=%d", t.name, n, perPair), func(b *testing.B) {
					nodes := t.setup(b, n)
					for _, node := range nodes {
						serveEcho(b, node)
					}
					var sessions []view.Session
					for _, from := range nodes {
						for _, to := range nodes {
							if from == to {
								continue
							}
							for range perPair {
								sessions = append(sessions, openSession(b, from, to))
							}
						}
					}
					payload := make([]byte, 1<<10)
					lat := make([]time.Duration, b.N)
					var remaining atomic.Int64
					remaining.Store(int64(b.N))

					b.SetBytes(int64(2 * len(payload)))
					b.ReportAllocs()
					b.ResetTimer()
					var wg sync.WaitGroup
					for _, s := range sessions {
						wg.Go(func() {
							for i := remaining.Add(-1); i >= 0; i = remaining.Add(-1) {
								start := time.Now()
								if err := roundTrip(b.Context(), s, payload); err != nil {
									b.Error(err)
									return
								}
								lat[i] = time.Since(start)
							}
						})
					}
					wg.Wait()
					b.StopTimer()
					benchmark.ReportTPS(b)
					reportPercentiles(b, lat)
				})
			}
		}
	}
}

// reportPercentiles reports the median and 99th percentile of the given round-trip durations.
// The mean is already reported as ns/op.
func reportPercentiles(b *testing.B, lat []time.Duration) {
	b.Helper()
	if len(lat) == 0 {
		return
	}
	slices.Sort(lat)
	b.ReportMetric(float64(lat[len(lat)*50/100].Nanoseconds()), "p50-ns")
	b.ReportMetric(float64(lat[len(lat)*99/100].Nanoseconds()), "p99-ns")
}

// openSession opens a session from a to z and completes one exchange on it. The first exchange
// between two nodes also establishes the transport connection, which libp2p may not accept
// until its host has finished starting, so it is retried until replyTimeout.
func openSession(b *testing.B, a, z *node) view.Session {
	b.Helper()
	deadline := time.Now().Add(replyTimeout)
	for {
		s, err := a.NewSession("", "", z.Address, []byte(z.ID))
		require.NoError(b, err)
		err = roundTrip(b.Context(), s, []byte{0})
		if err == nil {
			return s
		}
		s.Close()
		if time.Now().After(deadline) {
			b.Fatalf("no reply from [%s]: %v", z.Address, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// closeSession tells the echo responder to end session s, then closes s.
func closeSession(ctx context.Context, s view.Session) {
	_ = s.Send(ctx, bye)
	s.Close()
}

// roundTrip sends payload on s and waits for the echo.
func roundTrip(ctx context.Context, s view.Session, payload []byte) error {
	if err := s.Send(ctx, payload); err != nil {
		return err
	}
	select {
	case msg, ok := <-s.Receive():
		if !ok {
			return errors.New("session closed")
		}
		if !bytes.Equal(msg.Payload, payload) {
			return errors.Errorf("echo mismatch: sent %d bytes, received %d", len(payload), len(msg.Payload))
		}
		return nil
	case <-time.After(replyTimeout):
		return errors.Errorf("no reply within %s", replyTimeout)
	}
}

// serveEcho makes n answer every session opened to it by echoing each message back, until the
// initiator sends bye or the benchmark ends.
func serveEcho(b *testing.B, n *node) {
	b.Helper()
	master, err := n.MasterSession()
	require.NoError(b, err)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	// Registered after the node's own cleanup, so it runs first: responders stop before the node does.
	b.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-master.Receive():
				if !ok {
					return
				}
				s, err := n.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil) //nolint:contextcheck // the session API takes no context; stream cleanup inside it is independent of this loop
				if err != nil {
					b.Error(err)
					continue
				}
				wg.Go(func() { echo(ctx, s, msg) })
			}
		}
	})
}

func echo(ctx context.Context, s view.Session, msg *view.Message) {
	defer s.Close()
	for !bytes.Equal(msg.Payload, bye) {
		if err := s.Send(ctx, msg.Payload); err != nil {
			return
		}
		var ok bool
		select {
		case <-ctx.Done():
			return
		case msg, ok = <-s.Receive():
			if !ok {
				return
			}
		}
	}
}

func byteSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%dMiB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%dKiB", n>>10)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
