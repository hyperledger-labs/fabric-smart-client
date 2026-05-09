/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"errors"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type stubEndorserClient struct {
	response *pb.ProposalResponse
	err      error
}

func (s *stubEndorserClient) ProcessProposal(context.Context, *pb.SignedProposal, ...grpc.CallOption) (*pb.ProposalResponse, error) {
	return s.response, s.err
}

func TestCollectResponsesIgnoresQueryPeerErrorsWhenSomePeersReply(t *testing.T) {
	t.Parallel()

	invoke := &Invoke{}
	responses, err := invoke.collectResponses([]pb.EndorserClient{
		&stubEndorserClient{response: &pb.ProposalResponse{Response: &pb.Response{Payload: []byte("ok")}}},
		&stubEndorserClient{err: errors.New("peer down")},
	}, &pb.SignedProposal{}, true)
	require.NoError(t, err)
	require.Len(t, responses, 1)
	require.Equal(t, []byte("ok"), responses[0].GetResponse().Payload)
}

func TestCollectResponsesReturnsErrorWhenAllQueryPeersFail(t *testing.T) {
	t.Parallel()

	invoke := &Invoke{}
	_, err := invoke.collectResponses([]pb.EndorserClient{
		&stubEndorserClient{err: errors.New("peer one down")},
		&stubEndorserClient{err: errors.New("peer two down")},
	}, &pb.SignedProposal{}, true)
	require.Error(t, err)
	require.ErrorContains(t, err, "peer one down")
	require.ErrorContains(t, err, "peer two down")
}

func TestCollectResponsesStillFailsForEndorsementOnPeerError(t *testing.T) {
	t.Parallel()

	invoke := &Invoke{}
	_, err := invoke.collectResponses([]pb.EndorserClient{
		&stubEndorserClient{response: &pb.ProposalResponse{Response: &pb.Response{Payload: []byte("ok")}}},
		&stubEndorserClient{err: errors.New("peer down")},
	}, &pb.SignedProposal{}, false)
	require.Error(t, err)
	require.ErrorContains(t, err, "peer down")
}
