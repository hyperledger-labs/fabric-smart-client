/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// fakeNotifier is a committerpb.SidecarServiceClient whose streams a test can break and
// reopen, standing in for a committer that restarts. Every OpenNotificationStream
// call gets a fresh fakeStream, published on opened.
type fakeNotifier struct {
	committerpb.SidecarServiceClient // nil; only OpenNotificationStream is used

	opened chan *fakeStream

	mu      sync.Mutex
	openErr error
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{opened: make(chan *fakeStream, 16)}
}

func (f *fakeNotifier) OpenNotificationStream(ctx context.Context, _ ...grpc.CallOption) (committerpb.SidecarService_OpenNotificationStreamClient, error) {
	f.mu.Lock()
	err := f.openErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s := &fakeStream{
		ctx:       ctx,
		requests:  make(chan *committerpb.NotificationRequest, 2*maxTxIDsPerRequest),
		responses: make(chan *committerpb.NotificationResponse),
		broken:    make(chan struct{}),
	}
	f.opened <- s
	return s, nil
}

// setOpenErr makes every later OpenNotificationStream fail with err, as a committer
// that is down does; nil lets them succeed again.
func (f *fakeNotifier) setOpenErr(err error) {
	f.mu.Lock()
	f.openErr = err
	f.mu.Unlock()
}

// next waits for the manager to open its next stream.
func (f *fakeNotifier) next(t *testing.T) *fakeStream {
	t.Helper()
	select {
	case s := <-f.opened:
		return s
	case <-time.After(timeout):
		t.Fatal("notification stream was not opened")
		return nil
	}
}

// fakeStream is one notification stream. Requests the manager sends land on requests;
// a test answers through respond and kills the stream with fail.
type fakeStream struct {
	grpc.ClientStream // nil; listen() uses only Send and Recv

	ctx       context.Context
	requests  chan *committerpb.NotificationRequest
	responses chan *committerpb.NotificationResponse
	broken    chan struct{}
	breakOnce sync.Once
}

func (s *fakeStream) Send(req *committerpb.NotificationRequest) error {
	select {
	case <-s.broken:
		return errors.New("stream broken")
	case s.requests <- req:
		return nil
	}
}

func (s *fakeStream) Recv() (*committerpb.NotificationResponse, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case <-s.broken:
		return nil, errors.New("stream broken")
	case r := <-s.responses:
		return r, nil
	}
}

// fail breaks the stream, as a committer restart does.
func (s *fakeStream) fail() { s.breakOnce.Do(func() { close(s.broken) }) }

// respond delivers resp to the manager.
func (s *fakeStream) respond(t *testing.T, resp *committerpb.NotificationResponse) {
	t.Helper()
	select {
	case s.responses <- resp:
	case <-time.After(timeout):
		t.Fatal("manager did not receive the response")
	}
}

// requested waits for the manager to ask for txID on this stream.
func (s *fakeStream) requested(t *testing.T, txID string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case req := <-s.requests:
			if slices.Contains(req.GetTxStatusRequest().GetTxIds(), txID) {
				return
			}
		case <-deadline:
			t.Fatalf("txID=%s was not requested on the stream", txID)
		}
	}
}

// requestedAll waits until n distinct txIDs have been requested on this stream, and
// returns the requests that carried them.
func (s *fakeStream) requestedAll(t *testing.T, n int) []*committerpb.NotificationRequest {
	t.Helper()
	var reqs []*committerpb.NotificationRequest
	seen := map[string]bool{}
	deadline := time.After(timeout)
	for len(seen) < n {
		select {
		case req := <-s.requests:
			reqs = append(reqs, req)
			for _, id := range req.GetTxStatusRequest().GetTxIds() {
				seen[id] = true
			}
		case <-deadline:
			t.Fatalf("only %d of %d txIDs requested on the stream", len(seen), n)
		}
	}
	return reqs
}
