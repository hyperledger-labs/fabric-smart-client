/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"crypto/tls"
	"errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality/fake"
	viewgrpc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

func TestNewFabricFinality(t *testing.T) {
	t.Parallel()
	logger := logging.MustGetLogger("test")

	tests := []struct {
		name    string
		channel string
		wantErr bool
	}{
		{
			name:    "EmptyChannel",
			channel: "",
			wantErr: true,
		},
		{
			name:    "ValidParameters",
			channel: "testchannel",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f, err := NewFabricFinality(logger, tt.channel, nil, nil, nil, 5*time.Second, true)
			if tt.wantErr {
				require.Error(t, err)
				require.Nil(t, f)
			} else {
				require.NoError(t, err)
				require.NotNil(t, f)
				require.Equal(t, tt.channel, f.Channel)
			}
		})
	}
}

func TestFabricFinality_IsFinal(t *testing.T) {
	t.Parallel()
	logger := logging.MustGetLogger("test")

	// Helper to create a marshaled envelope with a specific TxID
	createTxEnvelope := func(txID string) []byte {
		chdrRaw, _ := proto.Marshal(&common.ChannelHeader{TxId: txID})
		payloadRaw, _ := proto.Marshal(&common.Payload{
			Header: &common.Header{
				ChannelHeader: chdrRaw,
			},
		})
		envRaw, _ := proto.Marshal(&common.Envelope{
			Payload: payloadRaw,
		})
		return envRaw
	}

	tests := []struct {
		name          string
		useFiltered   bool
		peerErr       error
		deliverErr    error
		streamErr     error
		sendErr       error
		signErr       error
		recvRes       *pb.DeliverResponse
		recvErr       error
		timeout       time.Duration
		wantErr       bool
		expectedError string
	}{
		{
			name:        "SuccessFiltered",
			useFiltered: true,
			recvRes: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: &pb.FilteredBlock{
						Number: 100,
						FilteredTransactions: []*pb.FilteredTransaction{
							{
								Txid:             "tx1",
								TxValidationCode: pb.TxValidationCode_VALID,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "SuccessUnfiltered",
			useFiltered: false,
			recvRes: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_Block{
					Block: &common.Block{
						Header: &common.BlockHeader{Number: 100},
						Data:   &common.BlockData{Data: [][]byte{createTxEnvelope("tx1")}},
						Metadata: &common.BlockMetadata{
							Metadata: [][]byte{
								nil, nil, {byte(pb.TxValidationCode_VALID)},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:          "PeerClientError",
			peerErr:       errors.New("peer-error"),
			wantErr:       true,
			expectedError: "failed creating peer client",
		},
		{
			name:          "StreamError",
			useFiltered:   true,
			streamErr:     errors.New("stream-error"),
			wantErr:       true,
			expectedError: "stream-error",
		},
		{
			name:          "SignError",
			useFiltered:   true,
			signErr:       errors.New("sign-error"),
			wantErr:       true,
			expectedError: "sign-error",
		},
		{
			name:          "SendError",
			useFiltered:   true,
			sendErr:       errors.New("send-error"),
			wantErr:       true,
			expectedError: "send-error",
		},
		{
			name:          "DeliverResponseStatusError",
			useFiltered:   true,
			recvRes:       &pb.DeliverResponse{Type: &pb.DeliverResponse_Status{Status: common.Status_NOT_FOUND}},
			wantErr:       true,
			expectedError: "deliver completed with status (NOT_FOUND)",
		},
		{
			name:          "UnexpectedResponseType",
			useFiltered:   true,
			recvRes:       &pb.DeliverResponse{Type: nil}, // nil type is unexpected
			wantErr:       true,
			expectedError: "received unexpected response type",
		},
		{
			name:          "Timeout",
			useFiltered:   true,
			timeout:       100 * time.Millisecond,
			wantErr:       true,
			expectedError: "timed out waiting for committing txid",
		},
		{
			name:        "NotCommitted",
			useFiltered: true,
			recvRes: &pb.DeliverResponse{
				Type: &pb.DeliverResponse_FilteredBlock{
					FilteredBlock: &pb.FilteredBlock{
						Number: 100,
						FilteredTransactions: []*pb.FilteredTransaction{
							{
								Txid:             "tx1",
								TxValidationCode: pb.TxValidationCode_MVCC_READ_CONFLICT,
							},
						},
					},
				},
			},
			wantErr:       true,
			expectedError: "status is not valid: MVCC_READ_CONFLICT",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockConfig := &fake.ConfigService{}
			mockConfig.On("PickPeer", mock.Anything).Return(&viewgrpc.ConnectionConfig{Address: "peer1"})

			mockPeerClient := &fake.PeerClient{}
			mockPeerClient.On("Close").Return()
			mockPeerClient.On("Certificate").Return(tls.Certificate{})
			mockPeerClient.On("Address").Return("peer1")

			mockDeliverClient := &fake.DeliverClient{}
			mockPeerClient.On("DeliverClient").Return(mockDeliverClient, tt.deliverErr)

			mockStream := &fake.DeliverFilteredStream{}
			if tt.useFiltered {
				mockDeliverClient.On("DeliverFiltered", mock.Anything, mock.Anything).Return(mockStream, tt.streamErr)
			} else {
				mockDeliverClient.On("Deliver", mock.Anything, mock.Anything).Return(mockStream, tt.streamErr)
			}

			mockIdentity := &fake.SigningIdentity{}
			mockIdentity.On("Serialize").Return([]byte("creator"), nil)
			mockIdentity.On("Sign", mock.Anything).Return([]byte("signature"), tt.signErr)

			mockServices := &fake.Services{}
			mockServices.On("NewPeerClient", mock.Anything).Return(mockPeerClient, tt.peerErr)

			timeout := 5 * time.Second
			if tt.timeout > 0 {
				timeout = tt.timeout
			}

			f, err := NewFabricFinality(logger, "testchannel", mockConfig, mockServices, mockIdentity, timeout, tt.useFiltered)
			require.NoError(t, err)

			if tt.peerErr == nil && tt.deliverErr == nil && tt.streamErr == nil && tt.signErr == nil {
				mockStream.On("CloseSend").Return(nil)
				mockStream.On("Send", mock.Anything).Return(tt.sendErr)
				if tt.sendErr == nil {
					if tt.name == "Timeout" {
						mockStream.On("Recv").Return(nil, context.DeadlineExceeded).After(500 * time.Millisecond)
					} else {
						mockStream.On("Recv").Return(tt.recvRes, tt.recvErr)
					}
				}
			}

			err = f.IsFinal("tx1", "peer1")
			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedError != "" {
					require.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
