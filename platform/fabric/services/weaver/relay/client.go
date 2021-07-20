/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package relay

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"io"
	"time"

	"github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/common"
	"github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/relay"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	hash2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

var logger = flogging.MustGetLogger("fabric-sdk.weaver.relay")

type TimeFunc func() time.Time

type SigningIdentity interface {
	Serialize() ([]byte, error)

	Sign(msg []byte) ([]byte, error)
}

//go:generate counterfeiter -o mock/data_transfer_client.go -fake-name DataTransferClient . DataTransferClient

// DataTransferClient defines an interface that creates a client to communicate with the view service in a peer
type DataTransferClient interface {
	// CreateDataTransferClient creates a grpc connection and client to the relay server
	CreateDataTransferClient() (*grpc.ClientConn, relay.DataTransferClient, error)

	// Certificate returns tls client certificate
	Certificate() *tls.Certificate
}

// DataTransferClientImpl implements DataTransferClient interface
type DataTransferClientImpl struct {
	Address            string
	ServerNameOverride string
	GRPCClient         *grpc2.Client
}

func (pc *DataTransferClientImpl) CreateDataTransferClient() (*grpc.ClientConn, relay.DataTransferClient, error) {
	logger.Debugf("opening connection to [%s]", pc.Address)
	conn, err := pc.GRPCClient.NewConnection(pc.Address)
	if err != nil {
		logger.Errorf("failed creating connection to [%s]: [%s]", pc.Address, err)
		return conn, nil, errors.Wrapf(err, "failed creating connection to [%s]", pc.Address)
	}
	logger.Debugf("opening connection to [%s], done.", pc.Address)

	return conn, relay.NewDataTransferClient(conn), nil
}

func (pc *DataTransferClientImpl) Certificate() *tls.Certificate {
	cert := pc.GRPCClient.Certificate()
	return &cert
}

// client implements network.Client interface
type client struct {
	Address            string
	DataTransferClient DataTransferClient
	RandomnessReader   io.Reader
	Time               TimeFunc
	SigningIdentity    SigningIdentity
	hasher             hash2.Hasher
}

func NewClient(config *ClientConfig, sID SigningIdentity, hasher hash2.Hasher) (*client, error) {
	// create a grpc client for view peer
	grpcClient, err := grpc2.CreateGRPCClient(config.RelayServer)
	if err != nil {
		return nil, err
	}

	return &client{
		Address:          config.RelayServer.Address,
		RandomnessReader: rand.Reader,
		Time:             time.Now,
		DataTransferClient: &DataTransferClientImpl{
			Address:            config.RelayServer.Address,
			ServerNameOverride: config.RelayServer.ServerNameOverride,
			GRPCClient:         grpcClient,
		},
		SigningIdentity: sID,
		hasher:          hasher,
	}, nil
}

func (s *client) RequestState() (*common.Ack, error) {
	logger.Debugf("get data transfer client...")
	conn, client, err := s.DataTransferClient.CreateDataTransferClient()
	logger.Debugf("get data transfer client...done")
	if conn != nil {
		logger.Debugf("get data transfer client...got a connection")
		defer conn.Close()
	}
	if err != nil {
		logger.Errorf("failed creating data transfer client [%s]", err)
		return nil, errors.Wrap(err, "failed creating data transfer client")
	}
	ctx := context.Background()
	query := &common.Query{
		Policy:             nil,
		Address:            "",
		RequestingRelay:    "",
		RequestingNetwork:  "",
		Certificate:        "",
		RequestorSignature: "",
		Nonce:              "",
		RequestId:          "",
		RequestingOrg:      "",
	}

	logger.Debugf("process request state query [%s]", query.String())
	ack, err := client.RequestState(ctx, query)
	if err != nil {
		logger.Errorf("failed requesting state [%s]", err)
		return nil, errors.Wrap(err, "failed requesting state")
	}
	return ack, nil
}

func (s *client) Close() {
	// TODO:
}
