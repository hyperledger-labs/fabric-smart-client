/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

func TestNewClient_Returns_Error_For_Missing_Config(t *testing.T) {
	t.Parallel()

	c, err := NewClient(nil, stubSigningIdentity{}, noop.NewTracerProvider())

	require.Error(t, err)
	require.Nil(t, c)
	require.Contains(t, err.Error(), "missing client config")
}

func TestNewClient_Returns_Error_For_Missing_Connection_Config(t *testing.T) {
	t.Parallel()

	c, err := NewClient(&Config{}, stubSigningIdentity{}, noop.NewTracerProvider())

	require.Error(t, err)
	require.Nil(t, c)
	require.Contains(t, err.Error(), "missing fsc peer connection config")
}

func TestInitiate_Returns_Context_ID(t *testing.T) {
	t.Parallel()

	rawResponse, err := proto.Marshal(&protos.CommandResponse{
		Payload: &protos.CommandResponse_InitiateViewResponse{
			InitiateViewResponse: &protos.InitiateViewResponse{Cid: "ctx-123"},
		},
	})
	require.NoError(t, err)

	viewClient := &fakeViewServiceClient{
		processCommandResponse: &protos.SignedCommandResponse{Response: rawResponse},
	}
	transport := &fakeTransport{
		viewClient:  viewClient,
		certificate: &tls.Certificate{},
		createErr:   nil,
	}

	c := &client{
		ViewServiceClient: transport,
		RandomnessReader:  bytes.NewReader(make([]byte, 32)),
		Time:              func() time.Time { return time.Unix(1700000000, 0) },
		SigningIdentity:   stubSigningIdentity{},
		tracer:            noop.NewTracerProvider().Tracer("test"),
	}

	contextID, err := c.Initiate("myView", []byte("payload"))

	require.NoError(t, err)
	require.Equal(t, "ctx-123", contextID)
	require.NotNil(t, viewClient.lastCommand)

	command := &protos.Command{}
	err = proto.Unmarshal(viewClient.lastCommand.Command, command)
	require.NoError(t, err)
	require.Equal(t, "myView", command.GetInitiateView().GetFid())
	require.Equal(t, []byte("payload"), command.GetInitiateView().GetInput())
}

func TestInitiate_Returns_Error_When_Response_Is_Missing(t *testing.T) {
	t.Parallel()

	rawResponse, err := proto.Marshal(&protos.CommandResponse{})
	require.NoError(t, err)

	viewClient := &fakeViewServiceClient{
		processCommandResponse: &protos.SignedCommandResponse{Response: rawResponse},
	}
	transport := &fakeTransport{
		viewClient:  viewClient,
		certificate: &tls.Certificate{},
		createErr:   nil,
	}

	c := &client{
		ViewServiceClient: transport,
		RandomnessReader:  bytes.NewReader(make([]byte, 32)),
		Time:              func() time.Time { return time.Unix(1700000000, 0) },
		SigningIdentity:   stubSigningIdentity{},
		tracer:            noop.NewTracerProvider().Tracer("test"),
	}

	contextID, err := c.Initiate("myView", []byte("payload"))

	require.Error(t, err)
	require.Empty(t, contextID)
	require.Contains(t, err.Error(), "expected initiate view response")
}

func TestValidateClientConfig_Returns_Error_For_Missing_Connection_Config(t *testing.T) {
	t.Parallel()

	err := ValidateClientConfig(Config{})

	require.EqualError(t, err, "missing fsc peer connection config")
}

type stubSigningIdentity struct{}

func (s stubSigningIdentity) Serialize() ([]byte, error) {
	return []byte("creator"), nil
}

func (s stubSigningIdentity) Sign(message []byte) ([]byte, error) {
	return []byte("signature"), nil
}

type fakeViewServiceClient struct {
	processCommandResponse *protos.SignedCommandResponse
	processCommandErr      error
	lastCommand            *protos.SignedCommand
}

type fakeTransport struct {
	viewClient  protos.ViewServiceClient
	certificate *tls.Certificate
	createErr   error
}

func (f *fakeTransport) CreateViewClient() (*grpc.ClientConn, protos.ViewServiceClient, error) {
	return nil, f.viewClient, f.createErr
}

func (f *fakeTransport) Certificate() *tls.Certificate {
	return f.certificate
}

func (f *fakeViewServiceClient) ProcessCommand(_ context.Context, in *protos.SignedCommand, _ ...grpc.CallOption) (*protos.SignedCommandResponse, error) {
	f.lastCommand = in
	return f.processCommandResponse, f.processCommandErr
}

func (f *fakeViewServiceClient) StreamCommand(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[protos.SignedCommand, protos.SignedCommandResponse], error) {
	return nil, io.EOF
}

var (
	_ SigningIdentity          = stubSigningIdentity{}
	_ ViewServiceClient        = &fakeTransport{}
	_ protos.ViewServiceClient = &fakeViewServiceClient{}
)
