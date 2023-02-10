/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"time"

	"go.uber.org/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.ordering")

type Signer interface {
	// Sign the message
	Sign(msg []byte) ([]byte, error)
}

type ViewManager interface {
	InitiateView(view view.View) (interface{}, error)
}

type Network interface {
	Name() string
	PickOrderer() *grpc.ConnectionConfig
	Orderers() []*grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(blob interface{}) error
	Channel(name string) (driver.Channel, error)
	SignerService() driver.SignerService
	Config() *config.Config
}

type Transaction interface {
	Channel() string
	ID() string
	Creator() view.Identity
	Proposal() driver.Proposal
	ProposalResponses() []driver.ProposalResponse
	Bytes() ([]byte, error)
}

type Connection struct {
	Stream Broadcast
	Client *ordererClient
}

type service struct {
	sp      view2.ServiceProvider
	network Network
	metrics *metrics.Metrics

	connectionsCounter atomic.Int32
	connections        chan *Connection
}

func NewService(sp view2.ServiceProvider, network Network, poolSize int, metrics *metrics.Metrics) *service {
	return &service{
		sp:          sp,
		network:     network,
		metrics:     metrics,
		connections: make(chan *Connection, poolSize),
	}
}

func (o *service) Broadcast(blob interface{}) error {
	var env *common2.Envelope
	var err error
	switch b := blob.(type) {
	case Transaction:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new transaction to broadcast...")
		}
		env, err = o.createFabricEndorseTransactionEnvelope(b)
		if err != nil {
			return err
		}
	case *transaction.Envelope:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new envelope to broadcast (boxed)...")
		}
		env = b.Envelope()
	case *common2.Envelope:
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("new envelope to broadcast...")
		}
		env = blob.(*common2.Envelope)
	default:
		return errors.Errorf("invalid blob's type, got [%T]", blob)
	}

	return o.broadcastEnvelope(env)
}

func (o *service) createFabricEndorseTransactionEnvelope(tx Transaction) (*common2.Envelope, error) {
	ch, err := o.network.Channel(tx.Channel())
	if err != nil {
		return nil, errors.Wrapf(err, "failed getting channel [%s]", tx.Channel())
	}
	txRaw, err := tx.Bytes()
	if err != nil {
		return nil, errors.Wrapf(err, "failed marshalling tx [%s]", tx.ID())
	}
	err = ch.TransactionService().StoreTransaction(tx.ID(), txRaw)
	if err != nil {
		return nil, errors.Wrap(err, "failed storing tx")
	}

	// tx contains the proposal and the endorsements, assemble them in a fabric transaction
	signerID := tx.Creator()
	signer, err := o.network.SignerService().GetSigner(signerID)
	if err != nil {
		logger.Errorf("signer not found for %s while creating tx envelope for ordering [%s]", signerID.UniqueID(), err)
		return nil, errors.Wrapf(err, "signer not found for %s while creating tx envelope for ordering", signerID.UniqueID())
	}
	env, err := fabricutils.CreateEndorserSignedTX(&signerWrapper{signerID, signer}, tx.Proposal(), tx.ProposalResponses()...)
	if err != nil {
		return nil, errors.WithMessage(err, "could not assemble transaction")
	}

	return env, nil
}

func (o *service) broadcastEnvelope(env *common2.Envelope) error {
	// send the envelope for ordering
	var status *ab.BroadcastResponse
	var connection *Connection
	retries := o.network.Config().BroadcastNumRetries()
	retryInterval := o.network.Config().BroadcastRetryInterval()
	forceConnect := true
	var err error
	for i := 0; i < retries; i++ {
		if connection != nil {
			// throw away this connection
			o.discardConnection(connection)
		}
		if i > 0 {
			logger.Debugf("broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInterval)
		}
		if i > 0 || forceConnect {
			forceConnect = false
			connection, err = o.getConnection()
			if err != nil {
				logger.Warnf("failed to get connection to orderer [%s]", err)
				continue
			}
		}

		err = BroadcastSend(connection.Stream, env)
		if err != nil {
			continue
		}
		status, err = connection.Stream.Recv()
		if err != nil {
			continue
		}
		if status.GetStatus() != common2.Status_SUCCESS {
			o.releaseConnection(connection)
			return errors.Wrapf(err, "failed broadcasting, status %s", common2.Status_name[int32(status.GetStatus())])
		}

		labels := []string{
			"network", o.network.Name(),
		}
		o.metrics.OrderedTransactions.With(labels...).Add(1)
		o.releaseConnection(connection)

		return nil
	}
	o.discardConnection(connection)
	return errors.Wrap(err, "failed to send transaction to orderer")
}

func (o *service) getConnection() (*Connection, error) {
	select {
	case connection := <-o.connections:
		return connection, nil
	default:
		if o.connectionsCounter.Load() >= int32(cap(o.connections)) {
			return nil, errors.New("no connection available")
		}

		ordererConfig := o.network.PickOrderer()
		if ordererConfig == nil {
			return nil, errors.New("no orderer configured")
		}

		oClient, err := NewOrdererClient(ordererConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed creating orderer client for %s", ordererConfig.Address)
		}

		stream, err := oClient.NewBroadcast(context.Background())
		if err != nil {
			oClient.Close()
			return nil, errors.Wrapf(err, "failed creating orderer stream for %s", ordererConfig.Address)
		}

		o.connectionsCounter.Inc()
		return &Connection{
			Stream: stream,
			Client: oClient,
		}, nil
	}
}

func (o *service) discardConnection(connection *Connection) {
	if connection != nil {
		o.connectionsCounter.Dec()
		if connection.Stream != nil {
			if err := connection.Stream.CloseSend(); err != nil {
				logger.Warnf("failed to close connection to ordering [%s]", err)
			}
		}
		if connection.Client != nil {
			connection.Client.Close()
		}
	}
}

func (o *service) releaseConnection(connection *Connection) {
	select {
	case o.connections <- connection:
		return
	default:
		// if there is not enough space in the channel, then discuard the connection
		o.discardConnection(connection)
	}
}

type signerWrapper struct {
	creator view.Identity
	signer  Signer
}

func (s *signerWrapper) Sign(message []byte) ([]byte, error) {
	return s.signer.Sign(message)
}

func (s *signerWrapper) Serialize() ([]byte, error) {
	return s.creator, nil
}
