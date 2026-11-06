/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ordering

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/grpc/status"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

const bftSendRecvTimeout = 10 * time.Second

type bftOrdererState struct {
	pool  chan *Connection
	slots chan struct{}
}

// BFTBroadcaster sends each envelope to every orderer of a byzantine-fault-tolerant
// ordering service and succeeds once enough of them acknowledge it. It keeps a
// pool of broadcast streams per orderer.
//
// Broadcast returns as soon as the threshold is met, or as soon as too many
// orderers failed for it to be met, without waiting for the other orderers.
// Their sends finish in the background: an accepted envelope puts the stream
// back in the pool, a failed or rejected one discards it. Each send is bounded
// by bftSendRecvTimeout and is not cancelled by the caller's context, which
// only bounds the wait for a connection and for the threshold.
//
// An orderer whose send has not started when the attempt ends still gets the
// envelope if it has an idle stream in its pool or a free slot to open one. If
// it has neither, it is skipped: waiting for a slot would compete with the
// next attempt or broadcast and send the envelope late.
//
// It counts each envelope that reaches the threshold in OrderedTransactions, and
// reports the state of each orderer: connected when a stream is opened and
// after each envelope the orderer accepts, reconnecting when a failed stream is
// discarded. A send that times out counts as failed. A stream discarded after a
// rejection does not change the state: the orderer answered.
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

func (o *BFTBroadcaster) Broadcast(ctx context.Context, env *common.Envelope) error {
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
	f := (n - 1) / 3
	threshold := int(math.Ceil((float64(n) + float64(f) + 1) / 2.0))

	for i := range retries {
		if i > 0 {
			logger.DebugfContext(ctx, "broadcast, retry [%d]...", i)
			// wait a bit
			time.Sleep(retryInterval)
		}

		// Only getConnection uses attemptCtx. Once the attempt ends, a goroutine
		// that would have to wait for a slot gives up; see the type's godoc.
		attemptCtx, cancelAttempt := context.WithCancel(ctx)
		results := make(chan error, n)
		var success, failure int
		var errs []error

		for _, orderer := range orderers {
			go func(orderer *grpc.ConnectionConfig) {
				logger.DebugfContext(ctx, "get connection to [%s]", orderer.Address)
				connection, err := o.getConnection(attemptCtx, orderer)
				if err != nil {
					logger.WarnfContext(ctx, "failed to get connection to orderer [%s]: %s", orderer.Address, err)
					results <- errors.Wrapf(err, "failed connecting to [%s]", orderer.Address)
					return
				}

				logger.DebugfContext(ctx, "broadcast to [%s]", orderer.Address)
				// Broadcast may return before this send ends, and the caller may then
				// cancel ctx. SendAndRecv aborts the stream when its context is done, so
				// a send derived from ctx would count a slow but healthy orderer as
				// failed and tear its connection down. The send keeps ctx's values but
				// not its cancellation, and is bounded by bftSendRecvTimeout alone.
				sendRecvCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bftSendRecvTimeout)
				status, err := connection.SendAndRecv(sendRecvCtx, env)
				cancel()
				if err != nil {
					logger.ErrorfContext(ctx, "failed to get status after broadcast to [%s]: %s", orderer.Address, err.Error())
					connection.State.Set(connstate.Reconnecting)
					o.discardConnection(connection)
					results <- errors.Wrapf(err, "failed broadcasting to [%s]", orderer.Address)
					return
				}

				switch status.GetStatus() {
				case common.Status_SUCCESS:
					connection.State.Set(connstate.Connected)
					o.releaseConnection(connection, orderer)
					results <- nil
				default:
					o.discardConnection(connection)
					logger.ErrorfContext(ctx, "failed to get status after broadcast to [%s]: %s", orderer.Address, common.Status_name[int32(status.GetStatus())])
					results <- errors.Errorf("failed to get status after broadcast to [%s]: %s", orderer.Address, common.Status_name[int32(status.GetStatus())])
					return
				}
			}(orderer)
		}

		for range n {
			select {
			case err := <-results:
				if err == nil {
					success++
				} else {
					failure++
					errs = append(errs, err)
				}
			case <-ctx.Done():
				cancelAttempt()
				return errors.Wrapf(ctx.Err(), "broadcast aborted with [%d of %d] successes", success, threshold)
			}
			if success >= threshold {
				cancelAttempt()
				o.metrics.OrderedTransactions.With("network", o.ConfigService.NetworkName()).Add(1)
				return nil
			}
			if failure > n-threshold {
				break
			}
		}
		cancelAttempt()

		// fail
		logger.WarnfContext(ctx, "failed to broadcast, got [%d of %d] success and errs [%v], retry after a delay", success, threshold, errs)
	}

	return errors.New("failed to send transaction to the ordering service")
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
		return o.createConnectionWithSlot(to, state) //nolint:contextcheck // pooled connection: its stream must use a connection-scoped context, not this request's (see comment in createConnection)
	default:
	}

	select {
	case connection := <-state.pool:
		return connection, nil
	case <-state.slots:
		return o.createConnectionWithSlot(to, state) //nolint:contextcheck // same as above: pooled connection outlives this request
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

	state := connState(o.metrics).Tracker(o.ConfigService.NetworkName(), "", connstate.ComponentOrderer, to.Address)
	state.Set(connstate.Connected)
	return &Connection{
		Address: to.Address,
		Stream:  stream,
		Client:  client,
		Cancel:  cancel,
		State:   state,
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

func (*BFTBroadcaster) releaseSlot(state *bftOrdererState) {
	select {
	case state.slots <- struct{}{}:
	default:
		logger.Warnf("failed to release BFT orderer connection slot: slot channel is full")
	}
}
