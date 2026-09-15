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
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
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
	metrics             *Metrics
	lastBlockReceived   uint64
	bufferSize          int

	// commitRetries bounds how many times a block whose commit failed
	// transiently is replayed before the failure is treated as permanent. Zero
	// means a single attempt with no retry.
	commitRetries int

	// onFatal is invoked when a commit fails with classFatal, so that the
	// embedding application can decide whether to exit. A Delivery never exits
	// the process itself; see reportFatal.
	onFatal FatalHandler

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
	localMembership driver.LocalMembership,
	configService driver.ConfigService,
	peerManager Services,
	ledger driver.Ledger,
	callback driver.BlockCallback,
	vault Vault,
	waitForEventTimeout time.Duration,
	bufferSize int,
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
) (*Delivery, error) {
	if channelConfig == nil {
		return nil, errors.Errorf("expected channel config, got nil")
	}

	d := &Delivery{
		NetworkName:         networkName,
		channel:             channelConfig.ID(),
		channelConfig:       channelConfig,
		LocalMembership:     localMembership,
		ConfigService:       configService,
		Services:            peerManager,
		Ledger:              ledger,
		waitForEventTimeout: waitForEventTimeout,
		tracer: tracerProvider.Tracer("delivery", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{messageTypeLabel},
		})),
		callback:      callback,
		vault:         vault,
		bufferSize:    max(bufferSize, 1),
		commitRetries: max(channelConfig.DeliveryCommitRetries(), 0),
		metrics:       NewMetrics(metricsProvider),
		stop:          make(chan struct{}),
	}
	return d, nil
}

// WithFatalHandler installs the handler invoked when a commit fails in a way that
// leaves this node unable to trust its own committed state. It returns d so it
// can be chained onto New, and must be called before Run or Start.
//
// Without a handler such a failure stops the channel's delivery and is logged at
// ERROR, the same as any other unrecoverable failure. The handler exists for an
// application that would rather exit and be restarted by its supervisor than keep
// serving from state it cannot vouch for.
func (d *Delivery) WithFatalHandler(h FatalHandler) *Delivery {
	d.onFatal = h
	return d
}

// Start runs the delivery service in its own goroutine and returns
// immediately. The error returned by Run is discarded; use Run directly to
// observe it.
func (d *Delivery) Start(ctx context.Context) {
	go func() {
		_ = d.Run(ctx)
	}()
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
func (d *Delivery) Run(ctx context.Context) error { //nolint:contextcheck // documented nil-ctx fallback above (nil is treated as context.Background), not an ignored inherited context
	logger.Debugf("Running delivery service [%d]", ctr.Add(1))
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan blockResponse, d.bufferSize)
	go d.readBlocks(ch)
	go d.runReceiver(ctx, ch) //nolint:gosec // G118: documented nil-ctx fallback, not a dropped request context
	return d.untilStop()
}

// readBlocks invokes the callback for each block arriving on ch until the
// service is stopped.
//
// A callback failure is classified rather than treated uniformly (see
// failureClass): a transient one is retried on the same block, because the block
// stream is the only way this node learns its transactions were committed and
// losing it costs the channel every later block. A failure that a replay cannot
// clear stops the service, which is unavoidable — a node that cannot apply a
// block it has already accepted must not commit later ones over it — but the stop
// is counted and logged at ERROR so it reads as a fault rather than as an absence
// of traffic.
func (d *Delivery) readBlocks(ch <-chan blockResponse) {
	for {
		select {
		case b := <-ch:
			logger.Debugf("Invoking callback for block [%d]", b.block.Header.Number)
			stop, err := d.invokeCallback(b)
			if err != nil {
				d.failBlock(b.block.Header.Number, err)
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

// invokeCallback runs the callback for one block, retrying while the error it
// returns is transient. It returns the callback's stop flag and the error that
// ended the attempts, which is nil once the block is handled.
//
// The retry budget is bounded on purpose: an unbounded retry against a fault that
// turns out to be permanent is a silent stall, which is the failure mode this
// whole path exists to avoid. Exhausting it returns the last error, which
// readBlocks then treats as any other non-retryable failure. The budget comes
// from ChannelConfig.DeliveryCommitRetries and applies per block without
// compounding: a block that commits on its third attempt leaves the next block a
// full budget.
//
// Retrying is safe because a replayed block is a no-op for the committer:
// CommitEndorserTransaction checks vault status and skips transactions already
// marked valid or invalid, and CommitConfig skips a configuration already in the
// vault. The stream's own reconnect path relies on the same property, since
// GetStartPosition resumes from the last block received.
func (d *Delivery) invokeCallback(b blockResponse) (bool, error) {
	blockNum := b.block.Header.Number

	var stop bool
	var err error
	for attempt := 0; attempt <= d.commitRetries; attempt++ {
		if attempt > 0 {
			logger.Warnf("retrying block [%d] after transient commit failure, attempt [%d/%d]: [%v]",
				blockNum, attempt, d.commitRetries, err)
			select {
			case <-d.stop:
				return false, err
			case <-time.After(d.retryDelay()):
			}
		}

		// Checked before every attempt, including the first: a block already in
		// flight when the service is stopped must not be committed on the way
		// out, and a zero retry delay would otherwise let the select above fall
		// straight through to another attempt.
		select {
		case <-d.stop:
			return false, err
		default:
		}

		stop, err = d.callback(b.ctx, b.block)
		if classify(err) != classRetry {
			return stop, err
		}
		d.metrics.CommitRetries.Add(1)
	}

	logger.Errorf("block [%d] still failing after [%d] retries, giving up: [%v]", blockNum, d.commitRetries, err)

	// Escalated out of the retry class: a fault that survived every attempt is no
	// longer usefully called transient, and reporting it as retryable would hide
	// it from the class an operator alerts on.
	//
	// Joined rather than formatted in, so both sentinels stay matchable by
	// errors.Is — ErrRetriesExhausted for the class, and whatever the callback
	// returned for the cause. runBlockScan hands this error to application
	// callers of Scan, who branch on the cause with errors.Is the way this
	// package does everywhere else, so flattening it into message text with %v
	// would quietly break them. The repo's errors.Wrapf is cockroachdb's and does
	// not support %w, which renders as %!w(...) rather than wrapping.
	return stop, errors.Wrapf(
		errors.Join(ErrRetriesExhausted, err),
		"block [%d] failed after [%d] retries", blockNum, d.commitRetries,
	)
}

// retryDelay is how long to wait before replaying a block whose commit failed
// transiently. It reuses the stream's own reconnect delay: both are waiting for
// the same class of downstream fault to clear, and a second knob for it would
// have to be tuned against the first to mean anything.
//
// It is read per attempt rather than captured once per block so that a
// configuration change between attempts takes effect on the next wait.
func (d *Delivery) retryDelay() time.Duration {
	return d.channelConfig.DeliverySleepAfterFailure()
}

// failBlock stops the service after a callback failure that a replay cannot
// clear, recording the class so that an operator can tell a channel that has
// gone quiet apart from one that has no traffic.
func (d *Delivery) failBlock(blockNum uint64, err error) {
	class := classify(err)
	d.metrics.CommitFailures.With(failureClassLabel, class.String()).Add(1)
	logger.Errorf("callback failed for block [%d] on [%s:%s] with class [%s], stopping delivery: [%v]",
		blockNum, d.NetworkName, d.channel, class, err)

	// Stopped before the handler runs, so that a handler which blocks or exits
	// cannot leave this channel still consuming blocks it can no longer commit.
	d.Stop(err)
	if class == classFatal {
		d.reportFatal(err)
	}
}

// reportFatal hands a fatal commit failure to the handler installed for it, if
// any. A Delivery does not exit the process itself: it runs inside an embedding
// application that owns that decision, and killing the process from a library
// goroutine would take down every other channel and network with it.
func (d *Delivery) reportFatal(err error) {
	if d.onFatal == nil {
		logger.Errorf("fatal commit failure on [%s:%s] with no handler installed, delivery stopped: [%v]",
			d.NetworkName, d.channel, err)
		return
	}
	d.onFatal(d.NetworkName, d.channel, err)
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
					if !d.handleBlockResponse(deliveryCtx, span, r, ch, waitTime) { //nolint:contextcheck // deliveryCtx is a fresh root span per block-delivery attempt (context.Background(), see above), by design: block traces are independent per-attempt spans, not children of one span spanning the receiver's whole reconnect-loop lifetime
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
	if peerConnConf == nil {
		return nil, nil, errors.New("no peer configured for delivery")
	}

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
