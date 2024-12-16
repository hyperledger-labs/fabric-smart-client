/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/status"
)

type CFTBroadcaster struct {
	NetworkID     string
	ConfigService driver.ConfigService
	ClientFactory Services

	connSem     *semaphore.Weighted
	connections chan *Connection
	metrics     *metrics.Metrics
}

func NewCFTBroadcaster(configService driver.ConfigService, clientFactory Services, metrics *metrics.Metrics) *CFTBroadcaster {
	poolSize := configService.OrdererConnectionPoolSize()
	return &CFTBroadcaster{
		NetworkID:     configService.NetworkName(),
		ConfigService: configService,
		ClientFactory: clientFactory,
		connections:   make(chan *Connection, poolSize),
		connSem:       semaphore.NewWeighted(int64(poolSize)),
		metrics:       metrics,
	}
}

func (o *CFTBroadcaster) Broadcast(context context.Context, env *common2.Envelope) error {
	// send the envelope for ordering
	var status *ab.BroadcastResponse
	var connection *Connection
	retries := o.ConfigService.BroadcastNumRetries()
	retryInterval := o.ConfigService.BroadcastRetryInterval()
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
			connection, err = o.getConnection(context)
			if err != nil {
				logger.Warnf("failed to get connection to orderer [%s]", err)
				continue
			}
		}

		err = connection.Send(env)
		if err != nil {
			continue
		}
		status, err = connection.Recv()
		if err != nil {
			continue
		}
		if status.GetStatus() != common2.Status_SUCCESS {
			o.releaseConnection(connection)
			return errors.Wrapf(err, "failed broadcasting, status %s", common2.Status_name[int32(status.GetStatus())])
		}

		labels := []string{
			"network", o.NetworkID,
		}
		o.metrics.OrderedTransactions.With(labels...).Add(1)
		o.releaseConnection(connection)

		return nil
	}
	o.discardConnection(connection)
	return errors.Wrap(err, "failed to send transaction to orderer")
}

func (o *CFTBroadcaster) getConnection(ctx context.Context) (*Connection, error) {
	for {
		select {
		case connection := <-o.connections:
			// if there is a connection available, return it
			return connection, nil
		default:
			// Try to acquire the right to create a new connection.
			// If this fails, retry with an existing connection
			semContext, cancel := context.WithTimeout(ctx, 1*time.Second)
			if err := o.connSem.Acquire(semContext, 1); err != nil {
				cancel()
				break
			}
			cancel()

			// create connection
			to := o.ConfigService.PickOrderer()
			if to == nil {
				return nil, errors.New("no orderer configured")
			}

			client, err := o.ClientFactory.NewOrdererClient(*to)
			if err != nil {
				return nil, errors.Wrapf(err, "failed creating orderer client for %s", to.Address)
			}

			oClient, err := client.OrdererClient()
			if err != nil {
				rpcStatus, _ := status.FromError(err)
				return nil, errors.Wrapf(err, "failed to new a broadcast for %s, rpcStatus=%+v", to.Address, rpcStatus)
			}

			stream, err := oClient.Broadcast(ctx)
			if err != nil {
				client.Close()
				return nil, errors.Wrapf(err, "failed creating orderer stream for %s", to.Address)
			}

			return &Connection{
				Stream: stream,
				Client: client,
			}, nil
		}
	}
}

func (o *CFTBroadcaster) discardConnection(connection *Connection) {
	if connection != nil {
		o.connSem.Release(1)
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

func (o *CFTBroadcaster) releaseConnection(connection *Connection) {
	select {
	case o.connections <- connection:
		return
	default:
		// if there is not enough space in the channel, then discard the connection
		o.discardConnection(connection)
	}
}
