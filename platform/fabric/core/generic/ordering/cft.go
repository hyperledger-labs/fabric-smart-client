/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"time"

	common2 "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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

func (o *CFTBroadcaster) Broadcast(ctx context.Context, env *common2.Envelope) error {
	logger.DebugfContext(ctx, "Start CFT Broadcast")
	defer logger.DebugfContext(ctx, "End CFT Broadcast")

	retries := o.ConfigService.BroadcastNumRetries()
	retryInterval := o.ConfigService.BroadcastRetryInterval()
	// Every failure path assigns lastErr; the initial value only survives when
	// retries is zero, in which case no attempt is made at all and Broadcast
	// must still report a failure.
	lastErr := errors.Errorf("no attempt made to send transaction to orderer (retries=%d)", retries)
	for i := range retries {
		if i > 0 {
			logger.Debugf("broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInterval)
		}

		connection, err := o.getConnection(ctx)
		if err != nil {
			logger.WarnfContext(ctx, "failed to get connection to orderer [%s]", err)
			lastErr = err
			continue
		}

		status, err := sendAndRecv(connection, env)
		if err != nil {
			// the connection is broken, throw it away and retry with a fresh one
			logger.DebugfContext(ctx, "Discard connection")
			o.discardConnection(connection)
			lastErr = err
			continue
		}

		logger.DebugfContext(ctx, "Release connection")
		o.releaseConnection(connection)
		if status.GetStatus() != common2.Status_SUCCESS {
			// the orderer rejected the envelope, retrying will not help
			return errors.Errorf("failed broadcasting, status %s", common2.Status_name[int32(status.GetStatus())])
		}
		o.metrics.OrderedTransactions.With("network", o.NetworkID).Add(1)

		return nil
	}
	return errors.Wrap(lastErr, "failed to send transaction to orderer")
}

// sendAndRecv sends env on connection and waits for the orderer's acknowledgement.
func sendAndRecv(connection *Connection, env *common2.Envelope) (*ab.BroadcastResponse, error) {
	if err := connection.Send(env); err != nil {
		return nil, err
	}
	return connection.Recv()
}

func (o *CFTBroadcaster) getConnection(ctx context.Context) (*Connection, error) {
	defer logger.DebugfContext(ctx, "End get connection")
	for {
		logger.DebugfContext(ctx, "Try acquire connection")
		select {
		case connection := <-o.connections:
			logger.DebugfContext(ctx, "Acquired connection")
			// if there is a connection available, return it
			return connection, nil
		default:
			logger.DebugfContext(ctx, "Wait for semaphore")
			// Try to acquire the right to create a new connection.
			// If this fails, retry with an existing connection
			semContext, cancel := context.WithTimeout(ctx, 1*time.Second)
			if err := o.connSem.Acquire(semContext, 1); err != nil {
				logger.DebugfContext(ctx, "error while waiting: %w", err)
				cancel()
				break
			}
			cancel()
			logger.DebugfContext(ctx, "Got a semaphore")

			// create connection. The semaphore slot acquired above is only meant to be held
			// for the lifetime of a Connection, so every failure path from here on must
			// release it back before returning, or the pool permanently loses a slot.
			to := o.ConfigService.PickOrderer()
			if to == nil {
				o.connSem.Release(1)
				return nil, errors.New("no orderer configured")
			}

			client, err := o.ClientFactory.NewOrdererClient(*to)
			if err != nil {
				o.connSem.Release(1)
				return nil, errors.Wrapf(err, "failed creating orderer client for %s", to.Address)
			}

			oClient, err := client.OrdererClient()
			if err != nil {
				o.connSem.Release(1)
				client.Close()
				rpcStatus, _ := status.FromError(err)
				return nil, errors.Wrapf(err, "failed to new a broadcast for %s, rpcStatus=%+v", to.Address, rpcStatus)
			}

			// Get the broadcast stream to receive a reply of Acknowledgement for each common.Envelope in order, indicating success or type of failure.
			// Notice that this stream is shared, therefore its context must be something different from the context of the current broadcast request
			stream, err := oClient.Broadcast(context.Background()) //nolint:contextcheck // documented above: this stream is shared across broadcasts, so it deliberately does not use the current request's context
			if err != nil {
				o.connSem.Release(1)
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
