/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/stretchr/testify/require"
)

// mockBroadcast is a mock implementation of the Broadcast interface
type mockBroadcast struct {
	sendErr    error
	recvErr    error
	recvResp   *ab.BroadcastResponse
	recvDelay  time.Duration
	sendCalls  int
	recvCalls  int
	closeCalls int
	mu         sync.Mutex
}

func (m *mockBroadcast) Send(*common.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalls++
	return m.sendErr
}

func (m *mockBroadcast) Recv() (*ab.BroadcastResponse, error) {
	m.mu.Lock()
	m.recvCalls++
	delay := m.recvDelay
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	return m.recvResp, nil
}

func (m *mockBroadcast) CloseSend() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	return nil
}

func (m *mockBroadcast) getCalls() (send, recv, close int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sendCalls, m.recvCalls, m.closeCalls
}

// mockClient is a mock implementation of the OrdererClient interface
type mockClient struct {
	closed  bool
	address string
	mu      sync.Mutex
}

func (m *mockClient) Address() string {
	return m.address
}

func (m *mockClient) Certificate() tls.Certificate {
	return tls.Certificate{}
}

func (m *mockClient) OrdererClient() (ab.AtomicBroadcastClient, error) {
	return nil, nil
}

func (m *mockClient) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *mockClient) isClosed() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// TestConnection_Send tests the Send method
func TestConnection_Send(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sendErr     error
		expectError bool
	}{
		{
			name:        "successful send",
			sendErr:     nil,
			expectError: false,
		},
		{
			name:        "send with error",
			sendErr:     errors.New("send failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := &mockBroadcast{
				sendErr: tt.sendErr,
			}
			conn := &Connection{
				Stream: stream,
			}

			err := conn.Send(&common.Envelope{})

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.sendErr, err)
			} else {
				require.NoError(t, err)
			}

			send, _, _ := stream.getCalls()
			require.Equal(t, 1, send)
		})
	}

	t.Run("concurrent sends are serialized", func(t *testing.T) {
		t.Parallel()
		stream := &mockBroadcast{}
		conn := &Connection{
			Stream: stream,
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for range numGoroutines {
			go func() {
				defer wg.Done()
				_ = conn.Send(&common.Envelope{})
			}()
		}

		wg.Wait()

		send, _, _ := stream.getCalls()
		require.Equal(t, numGoroutines, send)
	})
}

// TestConnection_Recv tests the Recv method
func TestConnection_Recv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		recvErr     error
		recvResp    *ab.BroadcastResponse
		expectError bool
	}{
		{
			name:    "successful recv",
			recvErr: nil,
			recvResp: &ab.BroadcastResponse{
				Status: common.Status_SUCCESS,
			},
			expectError: false,
		},
		{
			name:        "recv with error",
			recvErr:     errors.New("recv failed"),
			recvResp:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := &mockBroadcast{
				recvErr:  tt.recvErr,
				recvResp: tt.recvResp,
			}
			conn := &Connection{
				Stream: stream,
			}

			resp, err := conn.Recv()

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, resp)
				require.Equal(t, tt.recvErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.recvResp, resp)
			}

			_, recv, _ := stream.getCalls()
			require.Equal(t, 1, recv)
		})
	}

	t.Run("concurrent recvs are serialized", func(t *testing.T) {
		t.Parallel()
		stream := &mockBroadcast{
			recvResp: &ab.BroadcastResponse{Status: common.Status_SUCCESS},
		}
		conn := &Connection{
			Stream: stream,
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for range numGoroutines {
			go func() {
				defer wg.Done()
				_, _ = conn.Recv()
			}()
		}

		wg.Wait()

		_, recv, _ := stream.getCalls()
		require.Equal(t, numGoroutines, recv)
	})
}

// TestConnection_SendAndRecv tests the SendAndRecv method
func TestConnection_SendAndRecv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		sendErr       error
		recvErr       error
		recvResp      *ab.BroadcastResponse
		expectError   bool
		expectedSends int
		expectedRecvs int
	}{
		{
			name:    "successful send and recv",
			sendErr: nil,
			recvErr: nil,
			recvResp: &ab.BroadcastResponse{
				Status: common.Status_SUCCESS,
			},
			expectError:   false,
			expectedSends: 1,
			expectedRecvs: 1,
		},
		{
			name:          "send fails",
			sendErr:       errors.New("send failed"),
			recvErr:       nil,
			recvResp:      nil,
			expectError:   true,
			expectedSends: 1,
			expectedRecvs: 0,
		},
		{
			name:          "recv fails",
			sendErr:       nil,
			recvErr:       errors.New("recv failed"),
			recvResp:      nil,
			expectError:   true,
			expectedSends: 1,
			expectedRecvs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := &mockBroadcast{
				sendErr:  tt.sendErr,
				recvErr:  tt.recvErr,
				recvResp: tt.recvResp,
			}
			conn := &Connection{
				Stream: stream,
			}

			resp, err := conn.SendAndRecv(t.Context(), &common.Envelope{})

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.recvResp, resp)
			}

			send, recv, _ := stream.getCalls()
			require.Equal(t, tt.expectedSends, send)
			require.Equal(t, tt.expectedRecvs, recv)
		})
	}

	// Context cancellation tests
	contextTests := []struct {
		name              string
		hasCancel         bool
		hasClient         bool
		expectCancelCall  bool
		expectClientClose bool
	}{
		{
			name:              "context cancelled with cancel and client",
			hasCancel:         true,
			hasClient:         true,
			expectCancelCall:  true,
			expectClientClose: true,
		},
		{
			name:              "context cancelled with nil cancel function",
			hasCancel:         false,
			hasClient:         true,
			expectCancelCall:  false,
			expectClientClose: true,
		},
		{
			name:              "context cancelled with nil client",
			hasCancel:         true,
			hasClient:         false,
			expectCancelCall:  true,
			expectClientClose: false,
		},
	}

	for _, tt := range contextTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream := &mockBroadcast{
				recvResp:  &ab.BroadcastResponse{Status: common.Status_SUCCESS},
				recvDelay: 200 * time.Millisecond,
			}

			var client *mockClient
			if tt.hasClient {
				client = &mockClient{}
			}

			cancelCalled := false
			var cancelFunc context.CancelFunc
			if tt.hasCancel {
				cancelFunc = func() {
					cancelCalled = true
				}
			}

			conn := &Connection{
				Stream: stream,
				Client: client,
				Cancel: cancelFunc,
			}

			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()

			resp, err := conn.SendAndRecv(ctx, &common.Envelope{})
			require.Error(t, err)
			require.Nil(t, resp)
			require.ErrorIs(t, err, context.DeadlineExceeded)

			if tt.expectCancelCall {
				require.True(t, cancelCalled, "cancel function should be called")
			}

			if tt.expectClientClose && client != nil {
				require.True(t, client.isClosed(), "client should be closed")
			}

			send, _, _ := stream.getCalls()
			require.Equal(t, 1, send)
		})
	}

	t.Run("concurrent send and recv operations", func(t *testing.T) {
		t.Parallel()
		stream := &mockBroadcast{
			recvResp: &ab.BroadcastResponse{Status: common.Status_SUCCESS},
		}
		conn := &Connection{
			Stream: stream,
		}

		const numGoroutines = 5
		var wg sync.WaitGroup
		wg.Add(numGoroutines)
		errChan := make(chan error, numGoroutines)

		for range numGoroutines {
			go func() {
				defer wg.Done()
				_, err := conn.SendAndRecv(t.Context(), &common.Envelope{})
				errChan <- err
			}()
		}

		wg.Wait()
		close(errChan)

		// All operations should succeed
		for err := range errChan {
			require.NoError(t, err)
		}

		send, recv, _ := stream.getCalls()
		require.Equal(t, numGoroutines, send)
		require.Equal(t, numGoroutines, recv)
	})
}

// TestConnection_Address tests the Address field
func TestConnection_Address(t *testing.T) {
	t.Parallel()

	t.Run("address is set correctly", func(t *testing.T) {
		t.Parallel()
		expectedAddress := "orderer-1:7050"
		conn := &Connection{
			Address: expectedAddress,
		}

		require.Equal(t, expectedAddress, conn.Address)
	})

	t.Run("address can be empty", func(t *testing.T) {
		t.Parallel()
		conn := &Connection{
			Address: "",
		}

		require.Empty(t, conn.Address)
	})
}

// TestConnection_ThreadSafety tests thread safety of Connection methods
func TestConnection_ThreadSafety(t *testing.T) {
	t.Parallel()

	t.Run("mixed operations are thread-safe", func(t *testing.T) {
		t.Parallel()
		stream := &mockBroadcast{
			recvResp: &ab.BroadcastResponse{Status: common.Status_SUCCESS},
		}
		conn := &Connection{
			Stream: stream,
		}

		const numGoroutines = 20
		var wg sync.WaitGroup
		wg.Add(numGoroutines * 3) // send, recv, and sendAndRecv

		// Concurrent sends
		for range numGoroutines {
			go func() {
				defer wg.Done()
				_ = conn.Send(&common.Envelope{})
			}()
		}

		// Concurrent recvs
		for range numGoroutines {
			go func() {
				defer wg.Done()
				_, _ = conn.Recv()
			}()
		}

		// Concurrent sendAndRecvs
		for range numGoroutines {
			go func() {
				defer wg.Done()
				_, _ = conn.SendAndRecv(t.Context(), &common.Envelope{})
			}()
		}

		wg.Wait()

		send, recv, _ := stream.getCalls()
		// sendAndRecv calls both send and recv, so total sends = numGoroutines (Send) + numGoroutines (SendAndRecv)
		require.Equal(t, numGoroutines*2, send)
		// total recvs = numGoroutines (Recv) + numGoroutines (SendAndRecv)
		require.Equal(t, numGoroutines*2, recv)
	})
}
