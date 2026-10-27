/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"fmt"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

const (
	channelConfigKey = "CHANNEL_CONFIG_ENV_BYTES"
	peerNamespace    = "_configtx"
)

var (
	// TODO: introduced due to a race condition in idemix.
	commitConfigMutex = &sync.Mutex{}
	logger            = logging.MustGetLogger()
	// ErrDiscardTX this error can be used to signal that a valid transaction should be discarded anyway
	ErrDiscardTX = errors.New("discard tx")
)

type (
	FinalityEvent   = driver2.FinalityEvent[driver.ValidationCode]
	FinalityManager = committer.FinalityManager[driver.ValidationCode]
)

type FabricFinality interface {
	IsFinal(ctx context.Context, txID string) error
}

type CommitTx struct {
	BlkNum driver2.BlockNum
	TxNum  driver2.TxNum
	TxID   driver2.TxID
	Type   common.HeaderType

	Raw      []byte
	Envelope *common.Envelope
}

type TransactionHandler = func(ctx context.Context, block *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error)

// OrderingService receives the consensus type and orderer endpoints whenever
// the committer applies or reloads a channel config that lists orderers.
type OrderingService interface {
	Configure(consensusType string, orderers []*grpc.ConnectionConfig) error
}

type Committer struct {
	ConfigService driver.ConfigService
	ChannelConfig driver.ChannelConfig

	Vault              driver.Vault
	EnvelopeService    driver.EnvelopeService
	TransactionFilters *committer.AggregatedTransactionFilter
	ProcessNamespaces  []string
	Ledger             driver.Ledger
	RWSetLoaderService driver.RWSetLoader
	ProcessorManager   driver.ProcessorManager
	MembershipService  driver.MembershipService
	OrderingService    OrderingService
	FabricFinality     FabricFinality
	metrics            *Metrics
	tracer             trace.Tracer
	TransactionManager driver.TransactionManager
	DependencyResolver DependencyResolver

	// configCaughtUp is set once catchUpConfig has succeeded. Only the
	// channel's block delivery calls Commit, one block at a time, so it needs
	// no lock.
	configCaughtUp bool

	events chan FinalityEvent

	logger committer.Logger

	// events
	FinalityManager *FinalityManager
	EventsPublisher events.Publisher

	Handlers      map[common.HeaderType]TransactionHandler
	QuietNotifier bool

	listeners      map[string][]chan FinalityEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration

	// commitBlock substitutes for one attempt at committing a block. It is nil in
	// production, where commitOnce runs the real path; a test sets it to drive
	// retryBlock's classification and budget without standing up a vault.
	commitBlock func(context.Context, *common.Block) error
}

func New(
	configService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	rwsetLoaderService driver.RWSetLoader,
	processorManager driver.ProcessorManager,
	eventsPublisher events.Publisher,
	channelMembershipService driver.MembershipService,
	orderingService OrderingService,
	fabricFinality FabricFinality,
	transactionManager driver.TransactionManager,
	dependencyResolver DependencyResolver,
	quiet bool,
	listenerManager driver.ListenerManager,
	tracerProvider tracing.Provider,
	metricsProvider metrics.Provider,
) *Committer {
	s := &Committer{
		ConfigService:      configService,
		ChannelConfig:      channelConfig,
		Vault:              vault,
		EnvelopeService:    envelopeService,
		TransactionFilters: committer.NewAggregatedTransactionFilter(),
		ProcessNamespaces:  []string{},
		Ledger:             ledger,
		RWSetLoaderService: rwsetLoaderService,
		ProcessorManager:   processorManager,
		MembershipService:  channelMembershipService,
		OrderingService:    orderingService,
		FinalityManager:    committer.NewFinalityManager[driver.ValidationCode](listenerManager, logger, vault, tracerProvider, channelConfig.FinalityEventQueueWorkers(), driver.Valid, driver.Invalid),
		EventsPublisher:    eventsPublisher,
		FabricFinality:     fabricFinality,
		TransactionManager: transactionManager,
		DependencyResolver: dependencyResolver,
		QuietNotifier:      quiet,
		metrics:            NewMetrics(tracerProvider, metricsProvider, configService.NetworkName(), channelConfig.ID()),
		tracer:             tracerProvider.Tracer("committer", tracing.WithMetricsOpts(tracing.MetricsOpts{})),
		logger:             logger.Named(fmt.Sprintf("[%s:%s]", configService.NetworkName(), channelConfig.ID())),
		listeners:          map[string][]chan FinalityEvent{},
		Handlers:           map[common.HeaderType]TransactionHandler{},
		pollingTimeout:     channelConfig.CommitterPollingTimeout(),
		events:             make(chan FinalityEvent, 2000),
	}
	s.Handlers[common.HeaderType_CONFIG] = s.HandleConfig
	s.Handlers[common.HeaderType_ENDORSER_TRANSACTION] = s.HandleEndorserTransaction
	return s
}

func (c *Committer) Start(ctx context.Context) error {
	go c.FinalityManager.Run(ctx)
	go c.runEventNotifiers(ctx)
	return nil
}

func (c *Committer) runEventNotifiers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-c.events:
			c.metrics.EventQueueLength.Add(-1)
			start := time.Now()
			c.notifyFinality(event)
			c.metrics.NotifyFinalityDuration.Observe(time.Since(start).Seconds())

			start = time.Now()
			c.FinalityManager.Post(event)
			c.metrics.PostFinalityDuration.Observe(time.Since(start).Seconds())

			start = time.Now()
			var driverVC driver.ValidationCode
			if peer.TxValidationCode(event.ValidationCode) == peer.TxValidationCode_VALID {
				driverVC = driver.Valid
			} else {
				driverVC = driver.Invalid
			}
			c.notifyTxStatus(event.TxID, driverVC, event.ValidationMessage)
			c.metrics.NotifyStatusDuration.Observe(time.Since(start).Seconds())
		}
	}
}

func (c *Committer) Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error) {
	vc, message, err := c.Vault.Status(ctx, txID)
	if err != nil {
		c.logger.Errorf("failed to get status of [%s]: %s", txID, err)
		return driver.Unknown, "", err
	}
	if vc == driver.Unknown {
		// give it a second chance
		exists, err := c.EnvelopeService.Exists(ctx, txID)
		if err != nil {
			return driver.Unknown, "", errors.WithMessagef(err, "failed checking stored envelope for [%s]", txID)
		}
		if exists {
			if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
				return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored envelope for [%s]", txID)
			}
			vc = driver.Busy
		}
	}
	return vc, message, nil
}

func (c *Committer) ProcessNamespace(nss ...string) error {
	c.ProcessNamespaces = append(c.ProcessNamespaces, nss...)
	return nil
}

func (c *Committer) AddTransactionFilter(sr driver.TransactionFilter) error {
	c.TransactionFilters.Add(sr)
	return nil
}

// DiscardTx marks txID Invalid in the vault, recording message as the reason.
//
// If the vault holds the transaction's read-write set (opened locally, or loaded
// from a stored envelope by Status), the set is released, then the status is
// written. Otherwise only the status is written: the transaction is unknown to
// this node, or its set was released by an earlier attempt whose status write
// failed, or lost in a restart, so a retry still reaches Invalid. A Valid or
// Invalid transaction without a set is left unchanged. Any failure is returned,
// so a transient vault failure reaches the commit retry.
func (c *Committer) DiscardTx(ctx context.Context, txID, message string) error {
	c.logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

	vc, _, err := c.Status(ctx, txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	if c.Vault.RWSExists(ctx, txID) {
		return errors.WithMessagef(c.Vault.DiscardTx(ctx, txID, message), "failed discarding tx [%s] in vault", txID)
	}
	if vc == driver.Valid || vc == driver.Invalid {
		c.logger.Debugf("transaction [%s] already has final status [%d], nothing to discard", txID, vc)
		return nil
	}
	return errors.WithMessagef(c.Vault.SetDiscarded(ctx, txID, message), "failed setting tx [%s] discarded in vault", txID)
}

func (c *Committer) CommitTX(ctx context.Context, txID string, block driver.BlockNum, indexInBlock driver.TxNum, envelope *common.Envelope) (err error) {
	c.logger.Debugf("Committing transaction [%s,%d,%d]", txID, block, indexInBlock)
	defer c.logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txID, block, indexInBlock, err)

	vc, _, err := c.Status(ctx, txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		c.logger.Debugf("[%s] is already valid", txID)
		return errors.Errorf("[%s] is already valid", txID)
	case driver.Invalid:
		// This should generate a panic
		c.logger.Debugf("[%s] is invalid", txID)
		return errors.Errorf("[%s] is invalid", txID)
	case driver.Unknown:
		return c.commitUnknown(ctx, txID, block, indexInBlock, envelope)
	case driver.Busy:
		return c.commitBusyTx(ctx, txID, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txID)
	}
}

func (c *Committer) AddFinalityListener(txID string, listener driver.FinalityListener) error {
	return c.FinalityManager.AddListener(txID, listener)
}

func (c *Committer) RemoveFinalityListener(txID string, listener driver.FinalityListener) error {
	c.FinalityManager.RemoveListener(txID, listener)
	return nil
}

// Commit commits the transactions in the block passed as argument.
//
// A transient failure — storage contention, a lost database connection, an
// expired context — is retried here, on this same block, because this is the
// component that can tell a fault worth retrying from one that will never clear:
// it owns the vault, the membership service and the channel configuration. Committing a block again is safe:
// CommitEndorserTransaction skips transactions already marked valid or invalid,
// and CommitConfig does not reapply a configuration this process already holds.
//
// The error returned is therefore final. Retrying it will not help, and a caller
// driving the commit — the channel's block delivery — should stop rather than
// inspect it. Every failure is counted before it is returned, so a stopped stream
// shows up as a fault rather than as an absence of traffic.
//
// A commit that ends because ctx ended is not counted. The caller's context ends
// when the delivery stops or the node shuts down, which is a deliberate stop
// rather than a fault, so counting it would raise the same alert as a channel that
// can no longer commit. The block is committed again once delivery resumes.
func (c *Committer) Commit(ctx context.Context, block *common.Block) error {
	newCtx, span := c.metrics.Commits.Start(ctx, "commit_block")
	defer span.End()

	err := c.retryBlock(newCtx, block)
	if err == nil {
		return nil
	}

	// Matched on the cause as well as on ctx: a deterministic failure that happens
	// to return while the node shuts down is still a fault and is counted.
	if ctx.Err() != nil && errors.HasCause(err, ctx.Err()) {
		c.logger.Warnf("commit of block [%d] stopped because its context ended: [%v]",
			block.GetHeader().GetNumber(), err)
		return err
	}

	class := classify(err)
	c.metrics.CommitFailures.With(failureClassLabel, class.String()).Add(1)
	c.logger.Errorf("commit failed for block [%d] with class [%s], not retrying further: [%v]",
		block.GetHeader().GetNumber(), class, err)
	return err
}

// retryBlock commits a block, retrying while the failure classifies as
// transient. It returns nil once the block is committed, or the error that ended
// the attempts.
//
// Two kinds of transient failure are retried differently:
//
//   - A lost database connection is retried until the block commits or ctx ends,
//     waiting between attempts with exponential backoff and full jitter, starting
//     at the retry delay and capped at maxConnectionLostDelay. A missing database
//     says nothing about the block, so no budget would be right for every outage;
//     ending ctx is what stops these retries.
//   - Every other transient failure, such as contention, is retried at most
//     `retries` times, with the retry delay between attempts. A fault that does
//     not clear within that budget is unlikely to be transient, and retrying it
//     forever would be a silent stall. Exhausting the budget wraps the last error
//     in ErrRetriesExhausted so that it escalates out of the retry class, keeping
//     the original cause matchable underneath.
//
// Attempts lost to the connection do not count against the budget, so a block
// whose failures alternate between the two kinds gets the full budget for its
// contention failures however long the outage lasts.
func (c *Committer) retryBlock(ctx context.Context, block *common.Block) error {
	blockNum := block.GetHeader().GetNumber()
	retries, retrySleep := c.retryPolicy()

	// abandon reports that the caller gave up before the block committed. Wrapped in
	// ErrRetriesExhausted so classify escalates out of classRetry — the cancelled
	// context would otherwise classify as retryable and leak that class to a caller
	// promised a final error. Both causes stay matchable.
	abandon := func(lastErr error) error {
		cause := ctx.Err()
		if lastErr != nil {
			cause = errors.Join(ctx.Err(), lastErr)
		}
		return errors.Wrapf(errors.Join(ErrRetriesExhausted, cause), "block [%d] abandoned", blockNum)
	}

	var err error
	// nextConnDelay is created on the first lost connection and kept for the rest
	// of the block, so the delay keeps growing across an outage even when other
	// failures come in between.
	var nextConnDelay func() time.Duration
	budgeted := 0
	for attempt := 1; ; attempt++ {
		// Checked before every attempt, including the first: a caller that has
		// already given up must not have this block committed on the way out, and
		// a zero retry delay would otherwise let the wait below fall straight
		// through to another attempt.
		if ctx.Err() != nil {
			return abandon(err)
		}

		err = c.commitOnce(ctx, block)
		if classify(err) != classRetry {
			return err
		}

		delay := retrySleep
		switch {
		case errors.HasCause(err, dbdriver.ErrConnectionLost):
			if nextConnDelay == nil {
				nextConnDelay = utils.NewBackoffRetryRunner(utils.Infinitely, retrySleep, maxConnectionLostDelay).NextDelay
			}
			delay = nextConnDelay()
		case budgeted == retries:
			// Wrapped rather than formatted in, so both stay matchable by errors.Is:
			// ErrRetriesExhausted for the class, and whatever failed for the cause.
			// The repo's errors.Wrapf is cockroachdb's and does not implement %w.
			return errors.Wrapf(
				errors.Join(ErrRetriesExhausted, err),
				"block [%d] failed after [%d] attempts, budget of [%d] retries exhausted", blockNum, attempt, retries,
			)
		default:
			budgeted++
		}

		c.metrics.CommitRetries.Add(1)
		c.logger.Warnf("retrying block [%d] in [%v] after transient commit failure, attempt [%d], budgeted retries [%d/%d]: [%v]",
			blockNum, delay, attempt, budgeted, retries, err)
		select {
		case <-ctx.Done():
			return abandon(err)
		case <-time.After(delay):
		}
	}
}

// commitOnce makes a single attempt at committing every transaction in the block,
// through commitBlock when a test has substituted one.
func (c *Committer) commitOnce(ctx context.Context, block *common.Block) error {
	if c.commitBlock != nil {
		return c.commitBlock(ctx, block)
	}
	return c.commitBlockOnce(ctx, block)
}

// commitBlockOnce makes a single attempt at committing every transaction in the
// block.
func (c *Committer) commitBlockOnce(ctx context.Context, block *common.Block) error {
	txs, err := unmarshalTxs(block)
	if err != nil {
		return errors.Wrapf(err, "[%s] unmarshal tx failed", c.ChannelConfig.ID())
	}

	if !c.configCaughtUp {
		if err := c.catchUpConfig(ctx); err != nil {
			return errors.WithMessage(err, "failed catching up on config transactions")
		}
		c.configCaughtUp = true
	}

	resolvedTxs := c.DependencyResolver.Resolve(txs)

	return c.commitTxs(ctx, resolvedTxs, block.Metadata)
}

func (c *Committer) commitTxs(ctx context.Context, parallelizableTxGroups ParallelExecutable[SerialExecutable[CommitTx]], blockMetadata *common.BlockMetadata) error {
	start := time.Now()
	var eg errgroup.Group
	eg.SetLimit(c.ChannelConfig.CommitParallelism())
	for _, txGroup := range parallelizableTxGroups {
		txs := txGroup
		eg.Go(func() error {
			for _, tx := range txs {
				newCtx, span := c.metrics.Commits.Start(ctx, "commit_tx")
				span.AddEvent("create_finality_event")

				start := time.Now()
				if handler, ok := c.Handlers[tx.Type]; !ok {
					c.logger.Debugf("[%s] Received unhandled transaction type: %s", c.ChannelConfig.ID(), tx.Type)
					c.metrics.HandlerDuration.With("status", "not_found").Observe(time.Since(start).Seconds())
					span.End()
				} else if event, err := handler(newCtx, blockMetadata, tx); err != nil {
					span.End()
					c.metrics.HandlerDuration.With("status", "failure").Observe(time.Since(start).Seconds())
					return errors.Wrapf(err, "failed calling handler for tx [%s]", tx.TxID)
				} else if event == nil {
					logger.Debugf("Ignore tx [%d:%d] [%s]", tx.BlkNum, tx.TxNum, tx.TxID)
				} else {
					c.logger.Debugf("commit transaction [%s] in filteredBlock [%d]", event.TxID, tx.BlkNum)
					span.AddEvent("call_finality_notifiers")
					c.metrics.HandlerDuration.With("status", "successful").Observe(time.Since(start).Seconds())
					start := time.Now()
					c.events <- *event
					c.metrics.EventQueueDuration.Observe(time.Since(start).Seconds())
					c.metrics.EventQueueLength.Add(1)
					span.End()
				}
			}
			return nil
		})
	}
	err := eg.Wait()
	c.metrics.BlockCommitDuration.Observe(time.Since(start).Seconds())
	return err
}

func unmarshalTxs(block *common.Block) ([]CommitTx, error) {
	txs := make([]CommitTx, len(block.Data.Data))
	for i, tx := range block.Data.Data {
		env, _, chdr, err := fabricutils.UnmarshalTx(tx)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal tx failed")
		}
		txs[i] = CommitTx{
			BlkNum:   block.Header.Number,
			TxNum:    uint64(i),
			TxID:     chdr.TxId,
			Type:     common.HeaderType(chdr.Type),
			Raw:      tx,
			Envelope: env,
		}
	}
	return txs, nil
}

// IsFinal takes in input a transaction id and waits for its confirmation
// with the respect to the passed context that can be used to set a deadline
// for the waiting time.
func (c *Committer) IsFinal(ctx context.Context, txID string) error {
	c.logger.Debugf("Is [%s] final?", txID)

	lastStatus := driver.Unknown
	for iter := 0; iter < c.ChannelConfig.CommitterFinalityNumRetries(); iter++ {
		vd, _, err := c.Status(ctx, txID)
		if err != nil {
			c.logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}
		lastStatus = vd

		switch vd {
		case driver.Valid:
			c.logger.Debugf("Tx [%s] is valid", txID)
			return nil
		case driver.Invalid:
			c.logger.Debugf("Tx [%s] is not valid", txID)
			return errors.Errorf("transaction [%s] is not valid", txID)
		case driver.Busy:
			c.logger.Debugf("Tx [%s] is known", txID)
			if iter < c.ChannelConfig.CommitterFinalityNumRetries()-1 {
				if err := sleep(ctx, c.ChannelConfig.CommitterFinalityUnknownTXTimeout()); err != nil {
					return errors.Wrapf(err, "stopped waiting for finality of [%s], last known status [%s]", txID, driver.StatusName(vd))
				}
			}
			continue
		case driver.Unknown:
			if iter <= 1 {
				c.logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				if err := sleep(ctx, c.ChannelConfig.CommitterFinalityUnknownTXTimeout()); err != nil {
					return errors.Wrapf(err, "stopped waiting for finality of [%s], last known status [%s]", txID, driver.StatusName(vd))
				}
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				c.logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txID, iter, debug.Stack())
			}
			err := c.FabricFinality.IsFinal(ctx, txID)
			if err == nil {
				c.logger.Debugf("Tx [%s] is final, remote check succeeded", txID)
				return nil
			}

			if vd, _, err2 := c.Status(ctx, txID); err2 == nil && vd == driver.Unknown {
				c.logger.Debugf("Tx [%s] is not final remotely, return [%s], [%d][%s]", txID, err, vd, err2)
				return err
			}
		default:
			return errors.Errorf("invalid status code, got [%s]", driver.StatusName(vd))
		}
	}

	c.logger.Debugf("Tx [%s] start listening...", txID)
	// Listen to the event
	return c.listenTo(ctx, txID, c.ChannelConfig.CommitterWaitForEventTimeout(), lastStatus)
}

// sleep waits for d, or returns ctx.Err() if ctx is done first.
func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (c *Committer) GetProcessNamespace() []string {
	return c.ProcessNamespaces
}

func (c *Committer) addListener(txID string, ch chan FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txID]
	if !ok {
		ls = []chan FinalityEvent{}
		c.listeners[txID] = ls
	}
	ls = append(ls, ch)
	c.listeners[txID] = ls
}

func (c *Committer) deleteListener(txID string, ch chan FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txID]
	if !ok {
		return
	}
	for i, l := range ls {
		if l == ch {
			ls = append(ls[:i], ls[i+1:]...)
			c.listeners[txID] = ls
			return
		}
	}
}

func (c *Committer) notifyFinality(event FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if event.Err != nil && !c.QuietNotifier {
		c.logger.Warnf("An error occurred for tx [%s], event: [%v]", event.TxID, event)
	}

	listeners := c.listeners[event.TxID]
	c.logger.DebugfContext(event.Ctx, "Notify the finality of [%s] to [%d] listeners, event: [%v]", event.TxID, len(listeners), event)
	for _, listener := range listeners {
		listener <- event
	}
}

// notifyChaincodeListeners notifies the chaincode event to the registered chaincode listeners.
func (c *Committer) notifyChaincodeListeners(event *ChaincodeEvent) {
	c.EventsPublisher.Publish(event)
}

// listenTo waits up to timeout for the finality of txID. lastStatus is the vault
// status the caller last observed; it is updated on every poll and named in the
// returned error. The error wraps ctx.Err() if ctx is done first, and
// driver.ErrFinalityTimeout if the timeout elapses.
func (c *Committer) listenTo(ctx context.Context, txID string, timeout time.Duration, lastStatus driver.ValidationCode) error {
	_, span := c.tracer.Start(ctx, "listen_for_transaction")
	defer span.End()

	c.logger.Debugf("Listen to finality of [%s]", txID)

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan FinalityEvent, 100)
	c.addListener(txID, ch)
	defer c.deleteListener(txID, ch)

	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		timeout := time.NewTimer(c.pollingTimeout)

		stop := false
		select {
		case <-ctx.Done():
			timeout.Stop()
			stop = true
		case event := <-ch:
			span.AddEvent("receive_channel_result")
			span.AddLink(trace.LinkFromContext(event.Ctx))                                                //nolint:contextcheck // event.Ctx is the finality event's own originating context (from the block-commit pipeline that produced it), linked here only for trace correlation; listenTo's own ctx (used above for ctx.Done()) is unrelated to when/how the tx was actually committed
			c.logger.DebugfContext(event.Ctx, "Got an answer to finality of [%s]: [%s]", txID, event.Err) //nolint:contextcheck // same as above: log against the event's originating context for correlation, not listenTo's ctx
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			span.AddEvent("check_vault_status")
			timeout.Stop()
			c.logger.Debugf("Got a timeout for finality of [%s], check the status", txID)
			vd, _, err := c.Status(ctx, txID)
			if err == nil {
				lastStatus = vd
				switch vd {
				case driver.Valid:
					c.logger.Debugf("Listen to finality of [%s]. VALID", txID)
					return nil
				case driver.Invalid:
					c.logger.Debugf("Listen to finality of [%s]. NOT VALID", txID)
					return errors.Errorf("transaction [%s] is not valid", txID)
				}
			}
			c.logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txID, err, vd)
		}
		if stop {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return errors.Wrapf(err, "stopped listening to transaction [%s], last known status [%s]", txID, driver.StatusName(lastStatus))
	}
	c.logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txID)
	return errors.Wrapf(driver.ErrFinalityTimeout, "failed to listen to transaction [%s] for timeout, last known status [%s]", txID, driver.StatusName(lastStatus))
}

func (c *Committer) applyConfigCommit(ctx context.Context, txID driver2.TxID, blockNumber driver2.BlockNum, seq driver2.TxNum, envelope []byte) error {
	c.logger.Debugf("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelConfig.ID(), blockNumber, seq)

	rws, err := c.Vault.NewRWSet(ctx, txID)
	if err != nil {
		return errors.Wrap(err, "cannot create rws for configtx")
	}
	defer rws.Done()

	key, err := compose.CreateCompositeKey(&strings.Builder{}, channelConfigKey, strconv.FormatUint(seq, 10))
	if err != nil {
		return errors.Wrap(err, "cannot create configtx rws key")
	}
	if err := rws.SetState(peerNamespace, key, envelope); err != nil {
		return errors.Wrap(err, "failed setting configtx state in rws")
	}
	rws.Done()
	// No discard on failure: the configuration is already in force, and the
	// config tx left Busy lets CommitConfig try again on retry or replay.
	if err := c.CommitTX(ctx, txID, blockNumber, 0, nil); err != nil {
		return errors.Wrap(err, "failed committing configtx rws")
	}
	return nil
}

func (c *Committer) commitBusyTx(ctx context.Context, txID string, block, indexInBlock uint64, envelope *common.Envelope) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	c.logger.DebugfContext(ctx, "[%s] committing", txID)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		c.logger.DebugfContext(ctx, "[%s] matching rwsets", txID)

		pt, headerType, err := c.TransactionManager.NewProcessedTransactionFromEnvelopePayload(envelope.Payload)
		if err != nil && headerType == -1 {
			c.logger.Errorf("[%s] failed to unmarshal envelope [%s]", txID, err)
			return err
		}
		if headerType == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			envExists := false
			if !c.Vault.RWSExists(ctx, txID) {
				if envExists, err = c.EnvelopeService.Exists(ctx, txID); err != nil {
					return errors.WithMessagef(err, "failed checking stored envelope for [%s]", txID)
				}
			}
			if envExists {
				// Then match rwsets
				c.logger.DebugfContext(ctx, "Extract stored env to vault")
				if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
					return errors.WithMessage(err, "failed to load stored envelope into the vault")
				}
				c.logger.DebugfContext(ctx, "Match RWSet")
				if err := c.Vault.Match(ctx, txID, pt.Results()); err != nil {
					c.logger.Errorf("[%s] rwsets do not match [%s]", txID, err)
					return errors.Wrapf(ErrDiscardTX, "[%s] rwsets do not match [%s]", txID, err)
				}
			} else {
				// Store it
				envelopeRaw, err := proto.Marshal(envelope)
				if err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				if err := c.EnvelopeService.StoreEnvelope(ctx, txID, envelopeRaw); err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(ctx, txID)
				if err != nil {
					return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
				}
				rws.Done()
			}
		}
	}

	// Post-Processes
	c.logger.DebugfContext(ctx, "[%s] post process rwset", txID)

	if err := c.postProcessTx(ctx, txID); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	c.logger.DebugfContext(ctx, "[%s] commit in vault", txID)
	if err := c.Vault.CommitTX(ctx, txID, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *Committer) commitUnknown(ctx context.Context, txID string, block, indexInBlock uint64, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	exists, err := c.EnvelopeService.Exists(ctx, txID)
	if err != nil {
		return errors.WithMessagef(err, "failed checking stored envelope for [%s]", txID)
	}
	if exists {
		return c.commitStoredEnvelope(ctx, txID, block, indexInBlock)
	}

	var envelopeRaw []byte
	if envelope != nil {
		// Store it
		envelopeRaw, err = proto.Marshal(envelope)
		if err != nil {
			return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
		}
	} else {
		// fetch envelope and store it
		envelopeRaw, err = c.fetchEnvelope(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting rwset for tx [%s]", txID)
		}
	}

	// shall we commit this unknown envelope
	// A transient failure is returned so the block is retried; any other
	// failure, like a rejection, skips a transaction this node does not track.
	if ok, err := c.filterUnknownEnvelope(ctx, txID, envelopeRaw); err != nil || !ok {
		if classify(err) == classRetry {
			return errors.WithMessagef(err, "failed filtering unknown envelope [%s]", txID)
		}
		c.logger.Debugf("[%s] unknown envelope will not be processed [%t,%v]", txID, ok, err)
		return nil
	}

	if err := c.EnvelopeService.StoreEnvelope(ctx, txID, envelopeRaw); err != nil {
		return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
	}
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(ctx, txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	rws.Done()
	return c.commitBusyTx(ctx, txID, block, indexInBlock, envelope)
}

func (c *Committer) commitStoredEnvelope(ctx context.Context, txID string, block, indexInBlock uint64) error {
	c.logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
		return err
	}
	// commit
	return c.commitBusyTx(ctx, txID, block, indexInBlock, nil)
}

func (c *Committer) fetchEnvelope(txID string) ([]byte, error) {
	pt, err := c.Ledger.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return nil, errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	return pt.Envelope(), nil
}

func (c *Committer) filterUnknownEnvelope(ctx context.Context, txID string, envelope []byte) (bool, error) {
	rws, _, err := c.RWSetLoaderService.GetInspectingRWSetFromEvn(ctx, txID, envelope)
	if err != nil {
		return false, errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	defer rws.Done()

	// check namespaces
	c.logger.DebugfContext(ctx, "[%s] contains namespaces [%v] or `initialized` key", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		if slices.Contains(c.ProcessNamespaces, ns) {
			c.logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
			return true, nil
		}

		// search a read dependency on a key containing "initialized"
		for pos := 0; pos < rws.NumReads(ns); pos++ {
			k, err := rws.GetReadKeyAt(ns, pos)
			if err != nil {
				return false, errors.WithMessagef(err, "Error reading key at [%d]", pos)
			}
			if strings.Contains(k, "initialized") {
				c.logger.Debugf("[%s] contains 'initialized' key [%v] in [%s], select it", txID, ns, rws.Namespaces())
				return true, nil
			}
		}
	}

	// check the filters
	if ok, err := c.TransactionFilters.Accept(txID, envelope); err != nil || ok {
		return ok, err
	}

	status, _, err := c.Status(ctx, txID)
	if err != nil {
		return false, err
	}
	return status == driver.Busy, nil
}

func (c *Committer) extractStoredEnvelopeToVault(ctx context.Context, txID driver2.TxID) error {
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(ctx, txID)
	if err != nil {
		// If another replica of the same node created the RWSet
		rws, _, err = c.RWSetLoaderService.GetRWSetFromETx(ctx, txID)
		if err != nil {
			return errors.WithMessagef(err, "failed to extract rws from envelope and etx [%s]", txID)
		}
	}
	rws.Done()
	return nil
}

func (c *Committer) postProcessTx(ctx context.Context, txID driver2.TxID) error {
	if err := c.ProcessorManager.ProcessByID(ctx, c.ChannelConfig.ID(), txID); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

func (c *Committer) notifyTxStatus(txID string, vc driver.ValidationCode, message string) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	sb, topic := compose.CreateTxTopic(c.ConfigService.NetworkName(), c.ChannelConfig.ID(), "")
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         topic,
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         compose.AppendAttributesOrPanic(sb, txID),
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
}
