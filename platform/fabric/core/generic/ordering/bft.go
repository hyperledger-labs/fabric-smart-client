/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	common2 "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/status"
)

type BFTBroadcaster struct {
	ConfigService driver.ConfigService
	ClientFactory Services

	connSem  *semaphore.Weighted
	metrics  *metrics.Metrics
	poolSize int

	connectionsLock sync.RWMutex
	connections     map[string]chan *Connection
}

func NewBFTBroadcaster(configService driver.ConfigService, cf Services, metrics *metrics.Metrics) *BFTBroadcaster {
	return &BFTBroadcaster{
		ConfigService: configService,
		ClientFactory: cf,
		connections:   map[string]chan *Connection{},
		connSem:       semaphore.NewWeighted(int64(configService.OrdererConnectionPoolSize())),
		metrics:       metrics,
		poolSize:      configService.OrdererConnectionPoolSize(),
	}
}

func (o *BFTBroadcaster) Broadcast(ctx context.Context, env *common2.Envelope) error {
	logger.DebugfContext(ctx, "Start BFT Broadcast")
	defer logger.DebugfContext(ctx, "End BFT Broadcast")
	// send the envelope for ordering
	retries := o.ConfigService.BroadcastNumRetries()
	retryInterval := o.ConfigService.BroadcastRetryInterval()
	orderers := o.ConfigService.Orderers()
	if len(orderers) < 4 {
		return errors.Errorf("not enough orderers, 4 minimum got [%d]", len(orderers))
	}

	n := len(orderers)
	f := (int(n) - 1) / 3
	threshold := int(math.Ceil((float64(n) + float64(f) + 1) / 2.0))

	for i := range retries {
		if i > 0 {
			logger.DebugfContext(ctx, "broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInterval)
		}

		counter := 0
		var errs []error
		var usedConnections []*Connection

		var wg sync.WaitGroup
		wg.Add(n)

		var lock sync.Mutex

		for _, orderer := range orderers {
			go func(orderer *grpc.ConnectionConfig) {
				defer wg.Done()

				logger.DebugfContext(ctx, "get connection to [%s]", orderer.Address)
				connection, err := o.getConnection(ctx, orderer)

				lock.Lock()
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "failed connecting to [%v]", orderer.Address))
					logger.WarnfContext(ctx, "failed to get connection to orderer [%s]", orderer.Address, err)
					lock.Unlock()
					return
				}

				lock.Unlock()

				logger.DebugfContext(ctx, "broadcast to [%s]", orderer.Address)
				err = connection.Send(env)
				if err != nil {
					logger.ErrorfContext(ctx, "failed to broadcast to [%s]: %s", orderer.Address, err.Error())
					lock.Lock()
					defer lock.Unlock()
					usedConnections = append(usedConnections, connection)
					return
				}
				status, err := connection.Recv()
				if err != nil {
					logger.ErrorfContext(ctx, "failed to get status after broadcast to [%s]: %s", orderer.Address, err.Error())
					lock.Lock()
					defer lock.Unlock()
					usedConnections = append(usedConnections, connection)
					return
				}

				lock.Lock()
				defer lock.Unlock()

				switch status.GetStatus() {
				case common2.Status_SUCCESS:
					o.releaseConnection(connection, orderer)
					counter++
				default:
					usedConnections = append(usedConnections, connection)
					logger.ErrorfContext(ctx, "failed to get status after broadcast to [%s]: %s", orderer.Address, common2.Status_name[int32(status.GetStatus())])
					errs = append(errs, fmt.Errorf("failed to get status after broadcast to [%s]: %s", orderer.Address, common2.Status_name[int32(status.GetStatus())]))
					return
				}
			}(orderer)

		}

		wg.Wait()

		// did we send to enough orderers?
		// if not, discard all connections
		if counter >= threshold {
			// success
			return nil
		}

		// fail
		logger.WarnfContext(ctx, "failed to broadcast, got [%d of %d] success and errs [%v], retry after a delay", counter, threshold, errs)
		// cleanup connections
		for _, connection := range usedConnections {
			o.discardConnection(connection)
		}
	}

	return errors.Errorf("failed to send transaction to the orderering service")
}

func (o *BFTBroadcaster) getConnection(ctx context.Context, to *grpc.ConnectionConfig) (*Connection, error) {
	pool := o.connectionPool(to.Address)
	for {
		select {
		case connection := <-pool:
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
			client, err := o.ClientFactory.NewOrdererClient(*to)
			if err != nil {
				return nil, errors.Wrapf(err, "failed creating orderer client for %s", to.Address)
			}

			oClient, err := client.OrdererClient()
			if err != nil {
				rpcStatus, _ := status.FromError(err)
				return nil, errors.Wrapf(err, "failed to new a broadcast, rpcStatus=%+v", rpcStatus)
			}

			// Get the broadcast stream to receive a reply of Acknowledgement for each common.Envelope in order, indicating success or type of failure.
			// Notice that this stream is shared, therefore its context must be something different from the context of the current broadcast request
			stream, err := oClient.Broadcast(context.Background())
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

func (o *BFTBroadcaster) discardConnection(connection *Connection) {
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

func (o *BFTBroadcaster) releaseConnection(connection *Connection, to *grpc.ConnectionConfig) {
	pool := o.connectionPool(to.Address)
	select {
	case pool <- connection:
		return
	default:
		// if there is not enough space in the channel, then discard the connection
		o.discardConnection(connection)
	}
}

func (o *BFTBroadcaster) connectionPool(id string) chan *Connection {
	o.connectionsLock.RLock()
	connections, ok := o.connections[id]
	o.connectionsLock.RUnlock()
	if !ok {
		o.connectionsLock.Lock()
		connections, ok = o.connections[id]
		if !ok {
			connections = make(chan *Connection, o.poolSize)
			o.connections[id] = connections
		}
		o.connectionsLock.Unlock()
	}

	return connections
}
