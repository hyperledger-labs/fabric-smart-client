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

	common2 "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

const bftSendRecvTimeout = 10 * time.Second

type bftOrdererState struct {
	pool  chan *Connection
	slots chan struct{}
}

type BFTBroadcaster struct {
	ConfigService driver.ConfigService
	ClientFactory Services

	statesLock sync.RWMutex
	states     map[string]*bftOrdererState

	metrics  *metrics.Metrics
	poolSize int
}

func NewBFTBroadcaster(configService driver.ConfigService, cf Services, metrics *metrics.Metrics) *BFTBroadcaster {
	poolSize := configService.OrdererConnectionPoolSize()
	if poolSize <= 0 {
		logger.Panicf("invalid ordering.connectionPoolSize [%d]: must be > 0", poolSize)
	}
	return &BFTBroadcaster{
		ConfigService: configService,
		ClientFactory: cf,
		states:        map[string]*bftOrdererState{},
		metrics:       metrics,
		poolSize:      poolSize,
	}
}

func (o *BFTBroadcaster) ordererState(addr string) *bftOrdererState {
	o.statesLock.RLock()
	state, ok := o.states[addr]
	o.statesLock.RUnlock()
	if ok {
		return state
	}

	o.statesLock.Lock()
	defer o.statesLock.Unlock()
	if state, ok = o.states[addr]; ok {
		return state
	}

	slots := make(chan struct{}, o.poolSize)
	for i := 0; i < o.poolSize; i++ {
		slots <- struct{}{}
	}
	state = &bftOrdererState{
		pool:  make(chan *Connection, o.poolSize),
		slots: slots,
	}
	o.states[addr] = state
	return state
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
				sendRecvCtx, cancel := context.WithTimeout(ctx, bftSendRecvTimeout)
				status, err := connection.SendAndRecv(sendRecvCtx, env)
				cancel()
				if err != nil {
					logger.ErrorfContext(ctx, "failed to get status after broadcast to [%s]: %s", orderer.Address, err.Error())
					lock.Lock()
					defer lock.Unlock()
					usedConnections = append(usedConnections, connection)
					errs = append(errs, errors.Wrapf(err, "failed broadcasting to [%v]", orderer.Address))
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

		// Discard errored connections on every path, including threshold success.
		for _, connection := range usedConnections {
			o.discardConnection(connection)
		}

		if counter >= threshold {
			// success
			return nil
		}

		// fail
		logger.WarnfContext(ctx, "failed to broadcast, got [%d of %d] success and errs [%v], retry after a delay", counter, threshold, errs)
	}

	return errors.Errorf("failed to send transaction to the ordering service")
}

func (o *BFTBroadcaster) getConnection(ctx context.Context, to *grpc.ConnectionConfig) (*Connection, error) {
	state := o.ordererState(to.Address)

	select {
	case connection := <-state.pool:
		return connection, nil
	default:
	}

	select {
	case <-state.slots:
		return o.createConnectionWithSlot(to, state)
	default:
	}

	select {
	case connection := <-state.pool:
		return connection, nil
	case <-state.slots:
		return o.createConnectionWithSlot(to, state)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (o *BFTBroadcaster) createConnectionWithSlot(to *grpc.ConnectionConfig, state *bftOrdererState) (*Connection, error) {
	connection, err := o.createConnection(to)
	if err != nil {
		o.releaseSlot(state)
		return nil, err
	}
	return connection, nil
}

func (o *BFTBroadcaster) createConnection(to *grpc.ConnectionConfig) (*Connection, error) {
	client, err := o.ClientFactory.NewOrdererClient(*to)
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating orderer client for %s", to.Address)
	}

	oClient, err := client.OrdererClient()
	if err != nil {
		client.Close()
		rpcStatus, _ := status.FromError(err)
		return nil, errors.Wrapf(err, "failed to new a broadcast, rpcStatus=%+v", rpcStatus)
	}

	// The broadcast stream is shared by pooled sends, so it uses a connection-scoped
	// context instead of the current request context. SendAndRecv cancels this
	// context when a request times out so a blocked Recv cannot wedge the process.
	streamCtx, cancel := context.WithCancel(context.Background())
	stream, err := oClient.Broadcast(streamCtx)
	if err != nil {
		cancel()
		client.Close()
		return nil, errors.Wrapf(err, "failed creating orderer stream for %s", to.Address)
	}

	return &Connection{
		Address: to.Address,
		Stream:  stream,
		Client:  client,
		Cancel:  cancel,
	}, nil
}

func (o *BFTBroadcaster) discardConnection(connection *Connection) {
	if connection != nil {
		if connection.Stream != nil {
			if err := connection.Stream.CloseSend(); err != nil {
				logger.Warnf("failed to close connection to ordering [%s]", err)
			}
		}
		if connection.Cancel != nil {
			connection.Cancel()
		}
		if connection.Client != nil {
			connection.Client.Close()
		}
		if connection.Address != "" {
			o.releaseSlot(o.ordererState(connection.Address))
		}
	}
}

func (o *BFTBroadcaster) releaseConnection(connection *Connection, to *grpc.ConnectionConfig) {
	state := o.ordererState(to.Address)
	select {
	case state.pool <- connection:
		return
	default:
		// if there is not enough space in the channel, then discard the connection
		o.discardConnection(connection)
	}
}

func (o *BFTBroadcaster) releaseSlot(state *bftOrdererState) {
	select {
	case state.slots <- struct{}{}:
	default:
		logger.Warnf("failed to release BFT orderer connection slot: slot channel is full")
	}
}
