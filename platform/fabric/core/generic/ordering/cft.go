/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	common2 "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
)

type CFTBroadcaster struct {
	Network     Network
	connSem     *semaphore.Weighted
	connections chan *Connection
	metrics     *metrics.Metrics
}

func NewCFTBroadcaster(network Network, poolSize int, metrics *metrics.Metrics) *CFTBroadcaster {
	return &CFTBroadcaster{
		Network:     network,
		connections: make(chan *Connection, poolSize),
		connSem:     semaphore.NewWeighted(int64(poolSize)),
		metrics:     metrics,
	}
}

func (o *CFTBroadcaster) Broadcast(context context.Context, env *common2.Envelope) error {
	// send the envelope for ordering
	var status *ab.BroadcastResponse
	var connection *Connection
	retries := o.Network.Config().BroadcastNumRetries()
	retryInterval := o.Network.Config().BroadcastRetryInterval()
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
			"network", o.Network.Name(),
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
			ordererConfig := o.Network.PickOrderer()
			if ordererConfig == nil {
				return nil, errors.New("no orderer configured")
			}

			oClient, err := NewClient(ordererConfig)
			if err != nil {
				return nil, errors.Wrapf(err, "failed creating orderer client for %s", ordererConfig.Address)
			}

			stream, err := oClient.NewBroadcast(ctx)
			if err != nil {
				oClient.Close()
				return nil, errors.Wrapf(err, "failed creating orderer stream for %s", ordererConfig.Address)
			}

			return &Connection{
				Stream: stream,
				Client: oClient,
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
