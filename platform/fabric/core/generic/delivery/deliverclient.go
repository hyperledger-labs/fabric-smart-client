/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"crypto/tls"
	"math"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TxEvent contains information for token transaction commit
type TxEvent struct {
	TxID         string
	Committed    bool
	Block        uint64
	IndexInBlock int
	CommitPeer   string
	Err          error
}

//go:generate counterfeiter -o mock/deliver_filtered.go -fake-name DeliverFiltered . DeliverFiltered

// DeliverFiltered defines the interface that abstracts deliver filtered grpc calls to peer
type DeliverFiltered interface {
	Send(*common.Envelope) error
	Recv() (*pb.DeliverResponse, error)
	CloseSend() error
}

// DeliverStream defines the interface that abstracts deliver grpc calls to peer
type DeliverStream interface {
	Send(*common.Envelope) error
	Recv() (*pb.DeliverResponse, error)
	CloseSend() error
}

//go:generate counterfeiter -o mock/deliver_client.go -fake-name DeliverClient . DeliverClient

// DeliverClient defines the interface to create a DeliverStream client
type DeliverClient interface {

	// NewDeliverFiltered returns a DeliverFiltered
	NewDeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (DeliverFiltered, error)

	// NewDeliver returns a DeliverStream
	NewDeliver(ctx context.Context, opts ...grpc.CallOption) (DeliverStream, error)

	// Certificate returns tls certificate for the deliver client to peer
	Certificate() *tls.Certificate
}

// deliverClient implements DeliverClient interface
type deliverClient struct {
	client services.PeerClient
}

func NewDeliverClient(client services.PeerClient) (DeliverClient, error) {
	return &deliverClient{
		client: client,
	}, nil
}

// NewDeliver creates a DeliverStream client
func (d *deliverClient) NewDeliver(ctx context.Context, opts ...grpc.CallOption) (DeliverStream, error) {
	dc, err := d.client.DeliverClient()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create deliver client for peer %s", d.client.Address())
	}
	df, err := dc.Deliver(ctx, opts...)
	if err != nil {
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a deliver filtered, rpcStatus=%+v", rpcStatus)
	}
	return df, nil
}

// NewDeliverFiltered creates a DeliverFiltered client
func (d *deliverClient) NewDeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (DeliverFiltered, error) {
	dc, err := d.client.DeliverClient()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create deliver client for peer %s", d.client.Address())
	}
	df, err := dc.DeliverFiltered(ctx, opts...)
	if err != nil {
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a deliver filtered, rpcStatus=%+v", rpcStatus)
	}
	return df, nil
}

func (d *deliverClient) Certificate() *tls.Certificate {
	cert := d.client.Certificate()
	return &cert
}

// CreateDeliverEnvelope creates a signed envelope with SeekPosition_Newest for block
func CreateDeliverEnvelope(channelID string, signingIdentity driver.SigningIdentity, cert *tls.Certificate, start *ab.SeekPosition) (*common.Envelope, error) {
	logger.Debugf("create delivery envelope starting from: [%s]", start)
	creator, err := signingIdentity.Serialize()
	if err != nil {
		return nil, err
	}

	// check for client certificate and compute SHA2-256 on certificate if present
	tlsCertHash, err := grpc2.GetTLSCertHash(cert)
	if err != nil {
		return nil, err
	}

	_, header, err := CreateHeader(common.HeaderType_DELIVER_SEEK_INFO, channelID, creator, tlsCertHash)
	if err != nil {
		return nil, err
	}

	stop := &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: math.MaxUint64,
			},
		},
	}

	seekInfo := &ab.SeekInfo{
		Start:    start,
		Stop:     stop,
		Behavior: ab.SeekInfo_BLOCK_UNTIL_READY,
	}

	raw, err := proto.Marshal(seekInfo)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling SeekInfo")
	}

	envelope, err := CreateEnvelope(raw, header, signingIdentity)
	if err != nil {
		return nil, err
	}

	return envelope, nil
}

func DeliverSend(df DeliverStream, envelope *common.Envelope) error {
	err := df.Send(envelope)
	if err := df.CloseSend(); err != nil {
		logger.Warnf("error closing deliver stream: %s", err)
	}
	return err
}

func DeliverReceive(df DeliverFiltered, address string, txid string, eventCh chan<- TxEvent) error {
	event := TxEvent{
		TxID:       txid,
		Committed:  false,
		CommitPeer: address,
	}

read:
	for {
		resp, err := df.Recv()
		if err != nil {
			event.Err = errors.WithMessagef(err, "error receiving deliver response from peer %s", address)
			break read
		}
		switch r := resp.Type.(type) {
		case *pb.DeliverResponse_FilteredBlock:
			filteredTransactions := r.FilteredBlock.FilteredTransactions
			for i, tx := range filteredTransactions {
				logger.Debugf("transaction [%s] in block [%d]", tx.Txid, r.FilteredBlock.Number)
				if tx.Txid == txid {
					if tx.TxValidationCode == pb.TxValidationCode_VALID {
						logger.Debugf("transaction [%s] in block [%d] is valid", tx.Txid, r.FilteredBlock.Number)
						event.Committed = true
						event.Block = r.FilteredBlock.Number
						event.IndexInBlock = i
					} else {
						logger.Debugf("transaction [%s] in block [%d] is not valid [%s]", tx.Txid, r.FilteredBlock.Number, tx.TxValidationCode)
						event.Err = errors.Errorf("transaction [%s] status is not valid: %s", tx.Txid, tx.TxValidationCode)
					}
					break read
				}
			}
		case *pb.DeliverResponse_Block:
			for i, tx := range r.Block.Data.Data {
				_, _, chdr, err := fabricutils.UnmarshalTx(tx)
				if err != nil {
					event.Err = errors.Wrapf(err, "error parsing transaction [%d,%d]", r.Block.Header.Number, i)
					break read
				} else if chdr.TxId == txid {
					event.Committed = true
					event.Block = r.Block.Header.Number
					event.IndexInBlock = i
					break read
				}
			}
		case *pb.DeliverResponse_Status:
			event.Err = errors.Errorf("deliver completed with status (%s) before txid %s received from peer %s", r.Status, txid, address)
			break read
		default:
			event.Err = errors.Errorf("received unexpected response type (%T) from peer %s", r, address)
			break read
		}
	}

	select {
	case eventCh <- event:
	default:
	}

	return event.Err
}

// DeliverWaitForResponse waits for either eventChan has value (i.e., response has been received) or ctx is timed out
// This function assumes that the eventCh is only for the specified txid
// If an eventCh is shared by multiple transactions, a loop should be used to listen to events from multiple transactions
func DeliverWaitForResponse(ctx context.Context, eventCh <-chan TxEvent, txid string) (bool, uint64, int, error) {
	select {
	case event := <-eventCh:
		if txid == event.TxID {
			return event.Committed, event.Block, event.IndexInBlock, event.Err
		}
		// should never get here
		return false, 0, 0, errors.Errorf("no event received for txid %s", txid)
	case <-ctx.Done():
		return false, 0, 0, errors.Errorf("timed out waiting for committing txid [%s]", txid)
	}
}

// CreateHeader creates common.Header for a token transaction
// tlsCertHash is for client TLS cert, only applicable when ClientAuthRequired is true
func CreateHeader(txType common.HeaderType, channelID string, creator []byte, tlsCertHash []byte) (string, *common.Header, error) {
	ts := timestamppb.Now()

	nonce, err := GetRandomNonce()
	if err != nil {
		return "", nil, err
	}

	txID := protoutil.ComputeTxID(nonce, creator)

	chdr := &common.ChannelHeader{
		Type:        int32(txType),
		ChannelId:   channelID,
		TxId:        txID,
		Epoch:       0,
		Timestamp:   ts,
		TlsCertHash: tlsCertHash,
	}
	chdrBytes, err := proto.Marshal(chdr)
	if err != nil {
		return "", nil, err
	}

	shdr := &common.SignatureHeader{
		Creator: creator,
		Nonce:   nonce,
	}
	shdrBytes, err := proto.Marshal(shdr)
	if err != nil {
		return "", nil, err
	}

	header := &common.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	}

	return txID, header, nil
}

// CreateEnvelope creates a common.Envelope with given tx bytes, header, and SigningIdentity
func CreateEnvelope(data []byte, header *common.Header, signingIdentity driver.SigningIdentity) (*common.Envelope, error) {
	payload := &common.Payload{
		Header: header,
		Data:   data,
	}

	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal common.Payload")
	}

	signature, err := signingIdentity.Sign(payloadBytes)
	if err != nil {
		return nil, err
	}

	txEnvelope := &common.Envelope{
		Payload:   payloadBytes,
		Signature: signature,
	}

	return txEnvelope, nil
}
