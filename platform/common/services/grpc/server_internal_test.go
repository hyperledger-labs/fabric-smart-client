/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/testpb"
)

type blockingEmptyService struct {
	testpb.UnimplementedEmptyServiceServer
	entered, release chan struct{}
}

func (s blockingEmptyService) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	close(s.entered)
	<-s.release
	return &testpb.Empty{}, nil
}

func TestStopGivesUpOnHandlerIgnoringCancellation(t *testing.T) { //nolint:paralleltest // overrides stopTimeout
	oldTimeout := stopTimeout
	stopTimeout = 50 * time.Millisecond
	t.Cleanup(func() { stopTimeout = oldTimeout })

	s, err := NewGRPCServer("127.0.0.1:0", ServerConfig{})
	require.NoError(t, err)
	svc := blockingEmptyService{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(svc.release) })
	testpb.RegisterEmptyServiceServer(s.Server(), svc)
	go func() { _ = s.Start() }()

	cc, err := grpc.NewClient(s.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	go func() { _, _ = testpb.NewEmptyServiceClient(cc).EmptyCall(t.Context(), &testpb.Empty{}) }()
	<-svc.entered

	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not give up on a handler that ignores cancellation")
	}
}
