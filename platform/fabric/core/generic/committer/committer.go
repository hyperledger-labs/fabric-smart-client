/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
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
	IsFinal(txID string, address string) error
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

type OrderingService interface {
	Configure(consensusType string, orderers []*grpc.ConnectionConfig) error
}

type OrdererConfig interface {
	ConsensusType() string
	Endpoints() []*grpc.ConnectionConfig
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
		metrics:            NewMetrics(tracerProvider, metricsProvider),
		tracer:             tracerProvider.Tracer("committer", tracing.WithMetricsOpts(tracing.MetricsOpts{Namespace: "core"})),
		logger:             logger.Named(fmt.Sprintf("[%s:%s]", configService.NetworkName(), channelConfig.ID())),
		listeners:          map[string][]chan FinalityEvent{},
		Handlers:           map[common.HeaderType]TransactionHandler{},
		pollingTimeout:     1 * time.Second,
		events:             make(chan FinalityEvent, 2000),
	}
	s.Handlers[common.HeaderType_CONFIG] = s.HandleConfig
	s.Handlers[common.HeaderType_ENDORSER_TRANSACTION] = s.HandleEndorserTransaction
	return s
}

func (c *Committer) Start(context context.Context) error {
	go c.FinalityManager.Run(context)
	go c.runEventNotifiers(context)
	return nil
}

func (c *Committer) runEventNotifiers(context context.Context) {
	for {
		select {
		case <-context.Done():
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
		if c.EnvelopeService.Exists(ctx, txID) {
			if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
				return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
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

func (c *Committer) DiscardTx(ctx context.Context, txID string, message string) error {

	c.logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

	vc, _, err := c.Status(ctx, txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(ctx, txID) {
			if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
				return errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
		} else {
			c.logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
			if err := c.Vault.SetDiscarded(ctx, txID, message); err != nil {
				c.logger.Errorf("failed setting tx discarded [%s] in vault: %s", txID, err)
			}
			return nil
		}
	}

	if err := c.Vault.DiscardTx(ctx, txID, message); err != nil {
		c.logger.Errorf("failed discarding tx [%s] in vault: %s", txID, err)
	}
	return nil
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
		return c.commit(ctx, txID, block, indexInBlock, envelope)
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

// Commit commits the transactions in the block passed as argument
func (c *Committer) Commit(ctx context.Context, block *common.Block) error {
	newCtx, span := c.metrics.Commits.Start(ctx, "commit_block")
	defer span.End()

	txs, err := unmarshalTxs(block)
	if err != nil {
		return errors.Wrapf(err, "[%s] unmarshal tx failed", c.ChannelConfig.ID())
	}

	resolvedTxs := c.DependencyResolver.Resolve(txs)

	return c.commitTxs(newCtx, resolvedTxs, block.Metadata)
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
			return nil, errors.Wrapf(err, "unmarshal tx failed")
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

	for iter := 0; iter < c.ChannelConfig.CommitterFinalityNumRetries(); iter++ {
		vd, _, err := c.Status(ctx, txID)
		if err != nil {
			c.logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}

		switch vd {
		case driver.Valid:
			c.logger.Debugf("Tx [%s] is valid", txID)
			return nil
		case driver.Invalid:
			c.logger.Debugf("Tx [%s] is not valid", txID)
			return errors.Errorf("transaction [%s] is not valid", txID)
		case driver.Busy:
			c.logger.Debugf("Tx [%s] is known", txID)
			continue
		case driver.Unknown:
			if iter <= 1 {
				c.logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				time.Sleep(c.ChannelConfig.CommitterFinalityUnknownTXTimeout())
			}

			c.logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txID, iter, debug.Stack())
			peerForFinality := c.ConfigService.PickPeer(driver.PeerForFinality).Address
			err := c.FabricFinality.IsFinal(txID, peerForFinality)
			if err == nil {
				c.logger.Debugf("Tx [%s] is final, remote check on [%s]", txID, peerForFinality)
				return nil
			}

			if vd, _, err2 := c.Status(ctx, txID); err2 == nil && vd == driver.Unknown {
				c.logger.Debugf("Tx [%s] is not final for remote [%s], return [%s], [%d][%s]", txID, peerForFinality, err, vd, err2)
				return err
			}
		default:
			return errors.Errorf("invalid status code, got [%c]", vd)
		}
	}

	c.logger.Debugf("Tx [%s] start listening...", txID)
	// Listen to the event
	return c.listenTo(ctx, txID, c.ChannelConfig.CommitterWaitForEventTimeout())
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

func (c *Committer) listenTo(ctx context.Context, txID string, timeout time.Duration) error {
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
			span.AddLink(trace.LinkFromContext(event.Ctx))
			c.logger.DebugfContext(event.Ctx, "Got an answer to finality of [%s]: [%s]", txID, event.Err)
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			span.AddEvent("check_vault_status")
			timeout.Stop()
			c.logger.Debugf("Got a timeout for finality of [%s], check the status", txID)
			vd, _, err := c.Status(ctx, txID)
			if err == nil {
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
	c.logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txID)
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txID)
}

func (c *Committer) commitConfig(ctx context.Context, txID driver2.TxID, blockNumber driver2.BlockNum, seq driver2.TxNum, envelope []byte) error {
	c.logger.Debugf("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelConfig.ID(), blockNumber, seq)

	rws, err := c.Vault.NewRWSet(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "cannot create rws for configtx")
	}
	defer rws.Done()

	key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(seq, 10)})
	if err != nil {
		return errors.Wrapf(err, "cannot create configtx rws key")
	}
	if err := rws.SetState(peerNamespace, key, envelope); err != nil {
		return errors.Wrapf(err, "failed setting configtx state in rws")
	}
	rws.Done()
	if err := c.CommitTX(ctx, txID, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(ctx, txID, err.Error()); err2 != nil {
			c.logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func (c *Committer) commit(ctx context.Context, txID string, block uint64, indexInBlock uint64, envelope *common.Envelope) error {
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
			if !c.Vault.RWSExists(ctx, txID) && c.EnvelopeService.Exists(ctx, txID) {
				// Then match rwsets
				c.logger.DebugfContext(ctx, "Extract stored env to vault")
				if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
					return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
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

func (c *Committer) commitUnknown(ctx context.Context, txID string, block uint64, indexInBlock uint64, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService.Exists(ctx, txID) {
		return c.commitStoredEnvelope(ctx, txID, block, indexInBlock)
	}

	var envelopeRaw []byte
	var err error
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
	if ok, err := c.filterUnknownEnvelope(ctx, txID, envelopeRaw); err != nil || !ok {
		c.logger.Debugf("[%s] unknown envelope will not be processed [%b,%s]", txID, ok, err)
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
	return c.commit(ctx, txID, block, indexInBlock, envelope)
}

func (c *Committer) commitStoredEnvelope(ctx context.Context, txID string, block uint64, indexInBlock uint64) error {
	c.logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(ctx, txID); err != nil {
		return err
	}
	// commit
	return c.commit(ctx, txID, block, indexInBlock, nil)
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
		for _, namespace := range c.ProcessNamespaces {
			if namespace == ns {
				c.logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
				return true, nil
			}
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

	status, _, _ := c.Status(ctx, txID)
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
