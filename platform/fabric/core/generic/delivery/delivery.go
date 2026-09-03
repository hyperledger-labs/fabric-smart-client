/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

var logger = logging.MustGetLogger()

// StartGenesis seeks the oldest block available on the ordering service, i.e.
// the genesis block. It is the fallback start position whenever the last
// processed block cannot be determined.
var StartGenesis = &ab.SeekPosition{
	Type: &ab.SeekPosition_Oldest{
		Oldest: &ab.SeekOldest{},
	},
}

// blockResponse pairs a block received from the peer's Deliver stream with the
// tracing context of the span that received it, so that the callback invoked on
// another goroutine stays attached to the same trace.
type blockResponse struct {
	ctx   context.Context
	block *cb.Block
}

type messageType = string

const (
	messageTypeLabel tracing.LabelName = "type"
	unknown          messageType       = "unknown"
	block            messageType       = "block"
	responseStatus   messageType       = "status"
	other            messageType       = "other"
)

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	// GetLastTxID returns the last transaction id committed
	GetLastTxID(ctx context.Context) (string, error)
	GetLastBlock(context.Context) (uint64, error)
}

// Services provides the peer clients that Delivery connects through.
type Services interface {
	// NewPeerClient returns a client for the peer described by cc.
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

// Delivery streams blocks from a Fabric peer's Deliver service and invokes a
// callback for each one. A Delivery is single-use: once stopped it cannot be
// restarted, and Run must not be called more than once. Stop may be called
// concurrently with Run and from any number of goroutines.
type Delivery struct {
	channel             string
	channelConfig       driver.ChannelConfig
	NetworkName         string
	LocalMembership     driver.LocalMembership
	ConfigService       driver.ConfigService
	Services            Services
	Ledger              driver.Ledger
	waitForEventTimeout time.Duration
	callback            driver.BlockCallback
	vault               Vault
	client              services.PeerClient
	tracer              trace.Tracer
	lastBlockReceived   uint64
	bufferSize          int

	// stop is closed exactly once, by Stop, to signal shutdown to every
	// goroutine started by Run. It carries no value: untilStop, readBlocks and
	// runReceiver all read it, so a value sent over it would be observed by
	// exactly one of them. The cause goes in stopErr instead.
	stop chan struct{}
	// stopOnce guards the close of stop.
	stopOnce sync.Once
	// stopErr holds the error passed to the first call to Stop, if any. It is
	// written before stop is closed, so any goroutine that observes the close
	// also observes the error.
	stopErr atomic.Pointer[error]
}

var ctr = atomic.Uint32{}

// New creates a Delivery for the given channel. It fails if channelConfig is
// nil. bufferSize bounds the queue of blocks awaiting the callback and is
// raised to 1 if not positive. The returned Delivery is inert until Run or
// Start is called.
func New(
	networkName string,
	channelConfig driver.ChannelConfig,
	LocalMembership driver.LocalMembership,
	ConfigService driver.ConfigService,
	PeerManager Services,
	Ledger driver.Ledger,
	callback driver.BlockCallback,
	vault Vault,
	waitForEventTimeout time.Duration,
	bufferSize int,
	tracerProvider tracing.Provider,
	_ metrics.Provider,
) (*Delivery, error) {
	if channelConfig == nil {
		return nil, errors.Errorf("expected channel config, got nil")
	}

	d := &Delivery{
		NetworkName:         networkName,
		channel:             channelConfig.ID(),
		channelConfig:       channelConfig,
		LocalMembership:     LocalMembership,
		ConfigService:       ConfigService,
		Services:            PeerManager,
		Ledger:              Ledger,
		waitForEventTimeout: waitForEventTimeout,
		tracer: tracerProvider.Tracer("delivery", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{messageTypeLabel},
		})),
		callback:   callback,
		vault:      vault,
		bufferSize: max(bufferSize, 1),
		stop:       make(chan struct{}),
	}
	return d, nil
}

// Start runs the delivery service in its own goroutine and returns
// immediately. The error returned by Run is discarded; use Run directly to
// observe it.
func (d *Delivery) Start(ctx context.Context) {
	go utils.IgnoreErrorFunc(func() error {
		return d.Run(ctx)
	})
}

// Stop shuts the delivery service down, reporting err as the cause. A nil err
// means a clean shutdown. Only the first call has any effect: err from later
// calls is discarded. Stop never blocks and is safe to call concurrently and
// after the service has already stopped.
func (d *Delivery) Stop(err error) {
	d.stopOnce.Do(func() {
		logger.Debugf("stop delivery with error [%v]", err)
		if err != nil {
			d.stopErr.Store(&err)
		}
		close(d.stop)
	})
}

// stopError returns the error passed to the first call to Stop, or nil if the
// service was stopped cleanly or is still running.
func (d *Delivery) stopError() error {
	if err := d.stopErr.Load(); err != nil {
		return *err
	}
	return nil
}

// Run streams blocks until the service is stopped, either by a call to Stop,
// by the callback reporting an error or asking to stop, or by ctx being
// cancelled. It blocks until then and returns the error that caused the
// shutdown, or nil for a clean stop. A nil ctx is treated as
// context.Background.
func (d *Delivery) Run(ctx context.Context) error {
	logger.Debugf("Running delivery service [%d]", ctr.Add(1))
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan blockResponse, d.bufferSize)
	go d.readBlocks(ch)
	go d.runReceiver(ctx, ch)
	return d.untilStop()
}

// readBlocks invokes the callback for each block arriving on ch until the
// service is stopped. It stops the service if the callback fails or asks to
// stop.
func (d *Delivery) readBlocks(ch <-chan blockResponse) {
	for {
		select {
		case b := <-ch:
			logger.Debugf("Invoking callback for block [%d]", b.block.Header.Number)
			stop, err := d.callback(b.ctx, b.block)
			if err != nil {
				logger.Errorf("callback errored for block [%d], stop delivery: [%v]", b.block.Header.Number, err)
				d.Stop(err)
				return
			}
			if stop {
				logger.Debugf("stopping delivery at block [%d]", b.block.Header.Number)
				d.Stop(nil)
				return
			}
		case <-d.stop:
			logger.Debugf("stopping block reader with err [%v]", d.stopError())
			return
		}
	}
}

// runReceiver maintains the Deliver stream to the peer, reconnecting on
// failure, and forwards received blocks to ch. It returns once the service is
// stopped; it stops the service itself when ctx is cancelled. It is a no-op if
// ctx or ch is nil.
func (d *Delivery) runReceiver(ctx context.Context, ch chan<- blockResponse) {
	if ctx == nil || ch == nil {
		return
	}
	var df DeliverStream
	var dfCancel context.CancelFunc
	var err error
	waitTime := d.channelConfig.DeliverySleepAfterFailure()
	counter := 0
	for {
		select {
		case <-d.stop:
			logger.Debugf("Stopped receiver")
			return
		default:
			select {
			case <-d.stop:
				logger.Debugf("Stopped receiver")
				if dfCancel != nil {
					dfCancel()
				}
				return
			case <-ctx.Done():
				logger.Debugf("Ctx done")
				// Time to cancel
				if dfCancel != nil {
					dfCancel()
				}
				d.Stop(errors.New("context done"))
			default:
				deliveryCtx, span := d.tracer.Start(context.Background(), "block_delivery", tracing.WithAttributes(tracing.String(messageTypeLabel, unknown)))
				if df == nil {
					logger.Debugf("deliver service [%s:%s], connecting...", d.NetworkName, d.channel)
					span.AddEvent("connect")
					df, dfCancel, err = d.connect(ctx)
					if err != nil {
						logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait %.1fs before reconnecting", d.NetworkName, d.channel, err, waitTime.Seconds())
						time.Sleep(waitTime)
						logger.Debugf("reconnecting to delivery service [%s:%s]", d.NetworkName, d.channel)
						span.RecordError(err)
						span.End()
						continue
					}
				}

				logger.Debugf("call receive, it is the [%d]-th time", counter)
				counter++
				span.AddEvent("wait_message")
				resp, err := df.Recv()
				span.AddEvent("received_message")
				if err != nil {
					if dfCancel != nil {
						dfCancel()
					}
					df = nil
					logger.Errorf("delivery service [%s:%s:%s], failed receiving response [%s]",
						d.client.Address(), d.NetworkName, d.channel,
						errors.WithMessagef(err, "error receiving deliver response from peer %s", d.client.Address()))
					span.RecordError(err)
					span.End()
					continue
				}

				switch r := resp.Type.(type) {
				case *pb.DeliverResponse_Block:
					span.SetAttributes(tracing.String(messageTypeLabel, block))
					if !d.handleBlockResponse(deliveryCtx, span, r, ch, waitTime) {
						if dfCancel != nil {
							dfCancel()
						}
						df = nil
						span.End()
						continue
					}
				case *pb.DeliverResponse_Status:
					span.SetAttributes(tracing.String(messageTypeLabel, responseStatus))
					if r.Status == cb.Status_NOT_FOUND {
						span.RecordError(errors.New("not found"))
						df = nil
						if dfCancel != nil {
							dfCancel()
						}
						logger.Warnf("delivery service [%s:%s:%s] status [%s], wait a few seconds before retrying", d.client.Address(), d.NetworkName, d.channel, r.Status)
						time.Sleep(waitTime)
					} else {
						logger.Warnf("delivery service [%s:%s:%s] status [%s]", d.client.Address(), d.NetworkName, d.channel, r.Status)
					}
				default:
					span.SetAttributes(tracing.String(messageTypeLabel, other))
					df = nil
					if dfCancel != nil {
						dfCancel()
					}
					logger.Errorf("delivery service [%s:%s:%s], got [%s]", d.client.Address(), d.NetworkName, d.channel, r)
				}
				span.End()
			}
		}
	}
}

// handleBlockResponse validates and dispatches a received block to ch.
// It returns false if the block is malformed (in which case the caller must
// tear down the current stream and retry), true if the block was handled.
func (d *Delivery) handleBlockResponse(ctx context.Context, span trace.Span, r *pb.DeliverResponse_Block, ch chan<- blockResponse, waitTime time.Duration) bool {
	if r.Block == nil || r.Block.Data == nil || r.Block.Header == nil || r.Block.Metadata == nil {
		logger.Debugf("deliver service [%s:%s:%s], received nil block", d.client.Address(), d.NetworkName, d.channel)
		span.RecordError(errors.New("nil block"))
		time.Sleep(waitTime)
		return false
	}

	logger.Debugf("delivery service [%s:%s:%s], commit block [%d]", d.client.Address(), d.NetworkName, d.channel, r.Block.Header.Number)
	d.lastBlockReceived = r.Block.Header.Number

	span.AddEvent(fmt.Sprintf("push_%d_to_channel", r.Block.Header.Number))
	logger.Debugf("Pushing block [%d] to channel with current length %d", r.Block.Header.Number, len(ch))
	ch <- blockResponse{
		ctx:   ctx,
		block: r.Block,
	}
	logger.Debugf("Pushed block [%d] to channel", r.Block.Header.Number)
	span.AddEvent("pushed_to_channel")
	return true
}

// untilStop blocks until the service is stopped and returns the error that
// caused it, or nil for a clean stop.
func (d *Delivery) untilStop() error {
	<-d.stop
	err := d.stopError()
	logger.Debugf("stopping delivery service with error [%v]", err)
	return err
}

// connect opens a Deliver stream to a peer picked for delivery and sends the
// seek envelope that positions it at the next block to process. It returns the
// stream and a cancel function that the caller must invoke to release it.
func (d *Delivery) connect(ctx context.Context) (DeliverStream, context.CancelFunc, error) {
	// first cleanup everything
	d.cleanup()

	peerConnConf := d.ConfigService.PickPeer(driver.PeerForDelivery)

	address := peerConnConf.Address
	logger.Debugf("connecting to deliver service at [%s] for [%s:%s]", address, d.NetworkName, d.channel)
	var err error
	d.client, err = d.Services.NewPeerClient(*peerConnConf)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed creating peer client for address [%s][%s:%s]", address, d.NetworkName, d.channel)
	}
	deliverClient, err := NewDeliverClient(d.client)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get deliver client")
	}
	newCtx, cancel := context.WithCancel(ctx)
	stream, err := deliverClient.NewDeliver(newCtx)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrapf(err, "failed to get delivery stream")
	}

	blockEnvelope, err := CreateDeliverEnvelope(
		d.channel,
		d.LocalMembership.DefaultSigningIdentity(),
		deliverClient.Certificate(),
		d.GetStartPosition(newCtx),
	)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrap(err, "failed to create deliver envelope")
	}
	err = DeliverSend(stream, blockEnvelope)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrapf(err, "failed sending seek envelope to [%s]", address)
	}

	logger.Debugf("connected to deliver service at [%s]", address)
	return stream, cancel, nil
}

// GetStartPosition returns the position the Deliver stream should be seeked
// to. It prefers the last block this Delivery received, then the vault's last
// block, then the block holding the vault's last transaction, and falls back to
// StartGenesis when none of those can be determined.
func (d *Delivery) GetStartPosition(ctx context.Context) *ab.SeekPosition {
	if d.lastBlockReceived != 0 {
		logger.Debugf("restarting from the last block received [%d]", d.lastBlockReceived)

		return &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{
					Number: d.lastBlockReceived,
				},
			},
		}
	}

	logger.Debugf("no last block received set [%d], check last TxID in the vault", d.lastBlockReceived)

	lastBlock, err := d.vault.GetLastBlock(ctx)
	if err == nil && lastBlock != 0 {
		return SeekPosition(lastBlock)
	}

	logger.Debugf("failed to get last block [%s], try with last tx", err)
	lastTxID, err := d.vault.GetLastTxID(ctx)
	if err != nil {
		logger.Errorf("failed getting last transaction committed/discarded from the vault [%s], restarting from genesis", err)
		return StartGenesis
	}

	if len(lastTxID) != 0 && !strings.HasPrefix(lastTxID, committer.ConfigTXPrefix) {
		// Retrieve block from Fabric
		blockNumber, err := d.Ledger.GetBlockNumberByTxID(lastTxID)
		if err != nil {
			logger.Errorf("failed getting block number for transaction [%s], restart from genesis: error: %v", lastTxID, err)
			return StartGenesis
		}
		logger.Debugf("restarting from block [%d], tx [%s]", blockNumber, lastTxID)

		return SeekPosition(blockNumber)
	}

	return StartGenesis
}

// SeekPosition returns a seek position for the given block number.
func SeekPosition(blockNumber uint64) *ab.SeekPosition {
	return &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: blockNumber,
			},
		},
	}
}

// cleanup closes the current peer client, if any.
func (d *Delivery) cleanup() {
	if d.client != nil {
		d.client.Close()
	}
}
