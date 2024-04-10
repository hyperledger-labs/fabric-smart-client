/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type OrderingService interface {
	SetConfigOrderers(o channelconfig.Orderer, orderers []*grpc.ConnectionConfig) error
}

type CommitterService struct {
	NetworkName string
	ChannelName string

	Vault                    driver.Vault
	EnvelopeService          driver.EnvelopeService
	StatusReporters          []driver.StatusReporter
	ProcessNamespaces        []string
	Ledger                   driver.Ledger
	RWSetLoaderService       driver.RWSetLoader
	ProcessorManager         driver.ProcessorManager
	ChannelMembershipService *ChannelMembershipService
	OrderingService          OrderingService

	// events
	Subscribers      *events.Subscribers
	EventsSubscriber events.Subscriber
	EventsPublisher  events.Publisher
}

func NewCommitterService(
	networkName string,
	channelName string,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	RWSetLoaderService driver.RWSetLoader,
	processorManager driver.ProcessorManager,
	eventsSubscriber events.Subscriber,
	eventsPublisher events.Publisher,
	ChannelMembershipService *ChannelMembershipService,
	OrderingService OrderingService,
) *CommitterService {
	return &CommitterService{
		NetworkName:              networkName,
		ChannelName:              channelName,
		Vault:                    vault,
		EnvelopeService:          envelopeService,
		Ledger:                   ledger,
		RWSetLoaderService:       RWSetLoaderService,
		ProcessorManager:         processorManager,
		Subscribers:              events.NewSubscribers(),
		EventsSubscriber:         eventsSubscriber,
		EventsPublisher:          eventsPublisher,
		ChannelMembershipService: ChannelMembershipService,
		OrderingService:          OrderingService,
	}
}

func (c *CommitterService) Status(txID string) (driver.ValidationCode, string, []string, error) {
	vc, message, err := c.Vault.Status(txID)
	if err != nil {
		logger.Errorf("failed to get status of [%s]: %s", txID, err)
		return driver.Unknown, "", nil, err
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return driver.Unknown, "", nil, errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = driver.Busy
		} else {
			// check status reporter, if any
			for _, reporter := range c.StatusReporters {
				externalStatus, externalMessage, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					vc = externalStatus
					message = externalMessage
				}
			}
		}
	}
	return vc, message, nil, nil
}

func (c *CommitterService) ProcessNamespace(nss ...string) error {
	c.ProcessNamespaces = append(c.ProcessNamespaces, nss...)
	return nil
}

func (c *CommitterService) AddStatusReporter(sr driver.StatusReporter) error {
	c.StatusReporters = append(c.StatusReporters, sr)
	return nil
}

func (c *CommitterService) DiscardTx(txID string, message string) error {
	logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

	defer c.notifyTxStatus(txID, driver.Invalid, message)
	vc, _, deps, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
		} else {
			// check status reporter, if any
			found := false
			for _, reporter := range c.StatusReporters {
				externalStatus, _, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					found = true
					break
				}
			}
			if !found {
				logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
				return nil
			}
		}
	}

	if err := c.Vault.DiscardTx(txID, message); err != nil {
		logger.Errorf("failed discarding tx [%s] in vault: %s", txID, err)
	}
	for _, dep := range deps {
		if err := c.Vault.DiscardTx(dep, message); err != nil {
			logger.Errorf("failed discarding dependant tx [%s] of [%s] in vault: %s", dep, txID, err)
		}
	}
	return nil
}

func (c *CommitterService) CommitTX(txID string, block uint64, indexInBlock int, envelope *common.Envelope) (err error) {
	logger.Debugf("Committing transaction [%s,%d,%d]", txID, block, indexInBlock)
	defer logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txID, block, indexInBlock, err)
	defer func() {
		if err == nil {
			c.notifyTxStatus(txID, driver.Valid, "")
		}
	}()

	vc, _, _, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		logger.Debugf("[%s] is already valid", txID)
		return errors.Errorf("[%s] is already valid", txID)
	case driver.Invalid:
		// This should generate a panic
		logger.Debugf("[%s] is invalid", txID)
		return errors.Errorf("[%s] is invalid", txID)
	case driver.Unknown:
		return c.commitUnknown(txID, block, indexInBlock, envelope)
	case driver.Busy:
		return c.commit(txID, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txID)
	}
}

func (c *CommitterService) SubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.NetworkName, c.ChannelName, txID)
	l := &TxEventsListener{listener: listener}
	logger.Debugf("[%s] Subscribing to transaction status changes", txID)
	c.EventsSubscriber.Subscribe(topic, l)
	logger.Debugf("[%s] store mapping", txID)
	c.Subscribers.Set(topic, listener, l)
	logger.Debugf("[%s] Subscribing to transaction status changes done", txID)
	return nil
}

func (c *CommitterService) UnsubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.NetworkName, c.ChannelName, txID)
	l, ok := c.Subscribers.Get(topic, listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	el, ok := l.(events.Listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	c.Subscribers.Delete(topic, listener)
	c.EventsSubscriber.Unsubscribe(topic, el)
	return nil
}

func (c *CommitterService) GetProcessNamespace() []string {
	return c.ProcessNamespaces
}

// CommitConfig is used to validate and apply configuration transactions for a Channel.
func (c *CommitterService) CommitConfig(blockNumber uint64, raw []byte, env *common.Envelope) error {
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	c.ChannelMembershipService.ResourcesApplyLock.Lock()
	defer c.ChannelMembershipService.ResourcesApplyLock.Unlock()

	if env == nil {
		return errors.Errorf("Channel config found nil")
	}

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return errors.Wrapf(err, "cannot get payload from config transaction, block number [%d]", blockNumber)
	}

	ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	txid := committer.ConfigTXPrefix + strconv.FormatUint(ctx.Config.Sequence, 10)
	vc, _, err := c.Vault.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		logger.Infof("config block [%s] already committed, skip it.", txid)
		return nil
	case driver.Unknown:
		logger.Infof("config block [%s] not committed, commit it.", txid)
		// this is okay
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txid, vc)
	}

	var bundle *channelconfig.Bundle
	if c.ChannelMembershipService.Resources() == nil {
		// set up the genesis block
		bundle, err = channelconfig.NewBundle(c.ChannelName, ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to build a new bundle")
		}
	} else {
		configTxValidator := c.ChannelMembershipService.Resources().ConfigtxValidator()
		err := configTxValidator.Validate(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate config transaction, block number [%d]", blockNumber)
		}

		bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to create next bundle")
		}

		channelconfig.LogSanityChecks(bundle)
		if err := capabilitiesSupported(bundle); err != nil {
			return err
		}
	}

	if err := c.commitConfig(txid, blockNumber, ctx.Config.Sequence, raw); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	return c.applyBundle(bundle)
}

// TODO: introduced due to a race condition in idemix.
var commitConfigMutex = &sync.Mutex{}

func (c *CommitterService) ReloadConfigTransactions() error {
	c.ChannelMembershipService.ResourcesApplyLock.Lock()
	defer c.ChannelMembershipService.ResourcesApplyLock.Unlock()

	qe, err := c.Vault.NewQueryExecutor()
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer qe.Done()

	logger.Infof("looking up the latest config block available")
	var sequence uint64 = 0
	for {
		txID := committer.ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, _, err := c.Vault.Status(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting tx's status [%s]", txID)
		}
		logger.Infof("check config block at txID [%s], status [%v]...", txID, vc)
		done := false
		switch vc {
		case driver.Valid:
			logger.Infof("config block available, txID [%s], loading...", txID)

			key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(sequence, 10)})
			if err != nil {
				return errors.Wrapf(err, "cannot create configtx rws key")
			}
			envelope, err := qe.GetState(peerNamespace, key)
			if err != nil {
				return errors.Wrapf(err, "failed setting configtx state in rws")
			}
			env, err := protoutil.UnmarshalEnvelope(envelope)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			payload, err := protoutil.UnmarshalPayload(env.Payload)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
			if err != nil {
				return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks [%s]", txID)
			}

			var bundle *channelconfig.Bundle
			if c.ChannelMembershipService.Resources() == nil {
				// set up the genesis block
				bundle, err = channelconfig.NewBundle(c.ChannelName, ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to build a new bundle")
				}
			} else {
				configTxValidator := c.ChannelMembershipService.Resources().ConfigtxValidator()
				err := configTxValidator.Validate(ctx)
				if err != nil {
					return errors.Wrapf(err, "failed to validate config transaction [%s]", txID)
				}

				bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to create next bundle")
				}

				channelconfig.LogSanityChecks(bundle)
				if err := capabilitiesSupported(bundle); err != nil {
					return err
				}
			}

			if err := c.applyBundle(bundle); err != nil {
				return err
			}

			sequence = sequence + 1
			continue
		case driver.Unknown:
			if sequence == 0 {
				// Give a chance to 1, in certain setting the first block starts with 1
				sequence++
				continue
			}

			logger.Infof("config block at txID [%s] unavailable, stop loading", txID)
			done = true
		default:
			return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
		}
		if done {
			logger.Infof("loading config block done")
			break
		}
	}
	if sequence == 1 {
		logger.Infof("no config block available, must start from genesis")
		// no configuration block found
		return nil
	}
	logger.Infof("latest config block available at sequence [%d]", sequence-1)

	return nil
}

func (c *CommitterService) commitConfig(txID string, blockNumber uint64, seq uint64, envelope []byte) error {
	logger.Infof("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelName, blockNumber, seq)

	rws, err := c.Vault.NewRWSet(txID)
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
	if err := c.CommitTX(txID, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(txID, ""); err2 != nil {
			logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func (c *CommitterService) applyBundle(bundle *channelconfig.Bundle) error {
	c.ChannelMembershipService.ResourcesLock.Lock()
	defer c.ChannelMembershipService.ResourcesLock.Unlock()
	c.ChannelMembershipService.ChannelResources = bundle

	// update the list of orderers
	ordererConfig, exists := c.ChannelMembershipService.ChannelResources.OrdererConfig()
	if exists {
		logger.Debugf("[Channel: %s] Orderer config has changed, updating the list of orderers", c.ChannelName)

		var newOrderers []*grpc.ConnectionConfig
		orgs := ordererConfig.Organizations()
		for _, org := range orgs {
			msp := org.MSP()
			var tlsRootCerts [][]byte
			tlsRootCerts = append(tlsRootCerts, msp.GetTLSRootCerts()...)
			tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)
			for _, endpoint := range org.Endpoints() {
				logger.Debugf("[Channel: %s] Adding orderer endpoint: [%s:%s:%s]", c.ChannelName, org.Name(), org.MSPID(), endpoint)
				// TODO: load from configuration
				newOrderers = append(newOrderers, &grpc.ConnectionConfig{
					Address:           endpoint,
					ConnectionTimeout: 10 * time.Second,
					TLSEnabled:        true,
					TLSRootCertBytes:  tlsRootCerts,
				})
			}
		}
		if len(newOrderers) != 0 {
			logger.Debugf("[Channel: %s] Updating the list of orderers: (%d) found", c.ChannelName, len(newOrderers))
			if err := c.OrderingService.SetConfigOrderers(ordererConfig, newOrderers); err != nil {
				return err
			}
		} else {
			logger.Debugf("[Channel: %s] No orderers found in Channel config", c.ChannelName)
		}
	} else {
		logger.Debugf("no orderer configuration found in Channel config")
	}

	return nil
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}

// FetchEnvelope fetches from the ledger and stores the enveloped corresponding to the passed id
func (c *CommitterService) fetchEnvelope(txID string) ([]byte, error) {
	pt, err := c.Ledger.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return nil, errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	return pt.Envelope(), nil
}

func (c *CommitterService) commitUnknown(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService.Exists(txID) {
		return c.commitStoredEnvelope(txID, block, indexInBlock)
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
	if ok, err := c.filterUnknownEnvelope(txID, envelopeRaw); err != nil || !ok {
		logger.Debugf("[%s] unknown envelope will not be processed [%b,%s]", txID, ok, err)
		return nil
	}

	if err := c.EnvelopeService.StoreEnvelope(txID, envelopeRaw); err != nil {
		return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
	}
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	rws.Done()
	return c.commit(txID, block, indexInBlock, envelope)
}

func (c *CommitterService) filterUnknownEnvelope(txID string, envelope []byte) (bool, error) {
	rws, _, err := c.RWSetLoaderService.GetInspectingRWSetFromEvn(txID, envelope)
	if err != nil {
		return false, errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	defer rws.Done()

	logger.Debugf("[%s] contains namespaces [%v] or `initialized` key", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, namespace := range c.ProcessNamespaces {
			if namespace == ns {
				logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
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
				logger.Debugf("[%s] contains 'initialized' key [%v] in [%s], select it", txID, ns, rws.Namespaces())
				return true, nil
			}
		}
	}

	status, _, _, _ := c.Status(txID)
	return status == driver.Busy, nil
}

func (c *CommitterService) commitStoredEnvelope(txID string, block uint64, indexInBlock int) error {
	logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(txID); err != nil {
		return err
	}
	// commit
	return c.commitLocal(txID, block, indexInBlock, nil)
}

func (c *CommitterService) extractStoredEnvelopeToVault(txID string) error {
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
	if err != nil {
		// If another replica of the same node created the RWSet
		rws, _, err = c.RWSetLoaderService.GetRWSetFromETx(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed to extract rws from envelope and etx [%s]", txID)
		}
	}
	rws.Done()
	return nil
}

func (c *CommitterService) commit(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	logger.Debugf("[%s] is known.", txID)
	if err := c.commitLocal(txID, block, indexInBlock, envelope); err != nil {
		return err
	}
	return nil
}

func (c *CommitterService) commitLocal(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	logger.Debugf("[%s] committing", txID)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		logger.Debugf("[%s] matching rwsets", txID)

		pt, headerType, err := transaction.NewProcessedTransactionFromEnvelope(envelope)
		if err != nil && headerType == -1 {
			logger.Errorf("[%s] failed to unmarshal envelope [%s]", txID, err)
			return err
		}
		if headerType == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			if !c.Vault.RWSExists(txID) && c.EnvelopeService.Exists(txID) {
				// Then match rwsets
				if err := c.extractStoredEnvelopeToVault(txID); err != nil {
					return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
				}
				if err := c.Vault.Match(txID, pt.Results()); err != nil {
					logger.Errorf("[%s] rwsets do not match [%s]", txID, err)
					return errors.Wrapf(committer.ErrDiscardTX, "[%s] rwsets do not match [%s]", txID, err)
				}
			} else {
				// Store it
				envelopeRaw, err := proto.Marshal(envelope)
				if err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				if err := c.EnvelopeService.StoreEnvelope(txID, envelopeRaw); err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
				if err != nil {
					return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
				}
				rws.Done()
			}
		}
	}

	// Post-Processes
	logger.Debugf("[%s] post process rwset", txID)

	if err := c.postProcessTx(txID); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	logger.Debugf("[%s] commit in vault", txID)
	if err := c.Vault.CommitTX(txID, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *CommitterService) postProcessTx(txID string) error {
	if err := c.ProcessorManager.ProcessByID(c.ChannelName, txID); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

func (c *CommitterService) notifyTxStatus(txID string, vc driver.ValidationCode, message string) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	sb, topic := compose.CreateTxTopic(c.NetworkName, c.ChannelName, "")
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

type TxEventsListener struct {
	listener driver.TxStatusChangeListener
}

func (l *TxEventsListener) OnReceive(event events.Event) {
	tsc := event.Message().(*driver.TransactionStatusChanged)
	if err := l.listener.OnStatusChange(tsc.TxID, int(tsc.VC), tsc.ValidationMessage); err != nil {
		logger.Errorf("failed to notify listener for tx [%s] with err [%s]", tsc.TxID, err)
	}
}
