/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
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

func TestCallViewWithContext_Returns_Error_When_Response_Is_Missing(t *testing.T) {
	t.Parallel()

	// A response that answers the command but carries no CallViewResponse.
	c := newTestClient(&fakeProtosVSC{answer: &protos.CommandResponse{}})

	result, err := c.CallViewWithContext(context.Background(), "myView", []byte("payload"))

	require.ErrorContains(t, err, "expected call view response")
	require.Nil(t, result)
}

// A response is accepted only if its header names the hash of the command it answers.
func TestCallViewRejectsResponseToAnotherCommand(t *testing.T) {
	t.Parallel()
	transport := &fakeStreamClient{vsc: &fakeProtosVSC{cvrBytes: []byte("r"), commandHash: []byte("other")}}
	c := newTestClient(nil)
	c.Transport = transport

	_, err := c.CallView("v", nil)

	require.ErrorContains(t, err, "response does not answer the command sent")
	require.True(t, transport.closed, "the transport is released")
}

func TestNewClientRejectsUnknownTransport(t *testing.T) {
	t.Parallel()
	_, err := NewClient(&Config{ConnectionConfig: &grpc.ConnectionConfig{Address: "x:1"}, Transport: "carrier-pigeon"}, stubSigningIdentity{}, noop.NewTracerProvider())
	require.EqualError(t, err, "unknown view client transport [carrier-pigeon]")
}

func TestValidateClientConfig_Returns_Error_For_Missing_Connection_Config(t *testing.T) {
	t.Parallel()

	err := ValidateClientConfig(Config{})

	require.EqualError(t, err, "missing fsc peer connection config")
}

type stubSigningIdentity struct{}

func (stubSigningIdentity) Serialize() ([]byte, error) {
	return []byte("creator"), nil
}

func (stubSigningIdentity) Sign(_ []byte) ([]byte, error) {
	return []byte("signature"), nil
}

var _ SigningIdentity = stubSigningIdentity{}
