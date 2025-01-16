/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/committer"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var (
	logger              = logging.MustGetLogger("orion-sdk.core")
	waitForEventTimeout = 300 * time.Second
)

type network struct {
	ctx context.Context

	config *config2.Config
	name   string

	sessionManager     *SessionManager
	identityManager    *IdentityManager
	metadataService    driver.MetadataService
	transactionManager driver.TransactionManager
	envelopeService    driver.EnvelopeService
	vault              *Vault
	processorManager   driver.ProcessorManager
	transactionService driver.TransactionService
	finality           driver.Finality
	committer          driver.Committer
	deliveryService    driver.DeliveryService
}

func NewDB(
	ctx context.Context,
	endorseTxKVS driver.EndorseTxStore,
	metadataKVS driver.MetadataStore,
	envelopeKVS driver.EnvelopeStore,
	config *config2.Config,
	name string,
) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		name:   name,
		config: config,
	}
	ids, err := config.Identities()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load identities")
	}
	var dids []*driver.Identity
	for _, id := range ids {
		dids = append(dids, &driver.Identity{
			Name: id.Name,
			Cert: id.Cert,
			Key:  id.Key,
		})
	}
	n.identityManager = &IdentityManager{
		identities:      dids,
		defaultIdentity: dids[0].Name,
	}
	n.sessionManager = &SessionManager{
		config:          config,
		identityManager: n.identityManager,
	}
	n.metadataService = transaction.NewMetadataService(metadataKVS, name)
	n.envelopeService = transaction.NewEnvelopeService(envelopeKVS, name)
	n.transactionManager = transaction.NewManager(n.sessionManager)
	n.transactionService = transaction.NewEndorseTransactionService(endorseTxKVS, name)
	n.processorManager = rwset.NewProcessorManager(n, nil)

	return n, nil
}

func NewNetwork(
	ctx context.Context,
	endorseTxKVS driver.EndorseTxStore,
	metadataKVS driver.MetadataStore,
	envelopeKVS driver.EnvelopeStore,
	eventsPublisher events.Publisher,
	eventsSubscriber events.Subscriber,
	metricsProvider metrics.Provider,
	tracerProvider trace.TracerProvider,
	config *config2.Config,
	name string, drivers []driver2.NamedDriver,
	networkConfig driver.NetworkConfig,
	listenerManager driver.ListenerManager,
) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		name:   name,
		config: config,
	}
	ids, err := config.Identities()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load identities")
	}
	var dids []*driver.Identity
	for _, id := range ids {
		dids = append(dids, &driver.Identity{
			Name: id.Name,
			Cert: id.Cert,
			Key:  id.Key,
		})
	}
	n.identityManager = &IdentityManager{
		identities:      dids,
		defaultIdentity: dids[0].Name,
	}
	n.sessionManager = &SessionManager{
		config:          config,
		identityManager: n.identityManager,
	}
	n.metadataService = transaction.NewMetadataService(metadataKVS, name)
	n.envelopeService = transaction.NewEnvelopeService(envelopeKVS, name)
	n.transactionManager = transaction.NewManager(n.sessionManager)
	n.transactionService = transaction.NewEndorseTransactionService(endorseTxKVS, name)

	var d driver2.Driver
	for _, driver := range drivers {
		if driver.Name == n.config.VaultPersistenceType() {
			d = driver.Driver
			break
		}
	}
	if d == nil {
		return nil, errors.Errorf("driver %s not found in config", n.config.VaultPersistenceType())
	}

	persistence, err := db.OpenVersioned(d, name, db.NewPrefixConfig(n.config, n.config.VaultPersistencePrefix()))
	if err != nil {
		return nil, errors.Wrapf(err, "failed creating vault")
	}

	n.vault, err = NewVault(n, persistence, metricsProvider, tracerProvider)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create vault")
	}
	n.processorManager = rwset.NewProcessorManager(n, nil)

	committer, err := committer.New(
		name,
		n.processorManager,
		n.envelopeService,
		n.vault,
		nil,
		waitForEventTimeout,
		false,
		eventsPublisher,
		eventsSubscriber,
		tracerProvider,
		networkConfig,
		listenerManager,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create committer")
	}
	n.committer = committer

	finality, err := finality.NewService(committer, n, tracerProvider)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create finality service")
	}
	n.finality = finality

	deliveryService, err := delivery2.New(n, func(block *types.AugmentedBlockHeader) (bool, error) {
		if err := committer.Commit(block); err != nil {
			return true, err
		}
		return false, nil
	}, n.vault, waitForEventTimeout)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create delivery service")
	}
	n.deliveryService = deliveryService

	return n, nil
}

func (f *network) Name() string {
	return f.name
}

func (f *network) IdentityManager() driver.IdentityManager {
	return f.identityManager
}

func (f *network) SessionManager() driver.SessionManager {
	return f.sessionManager
}

func (f *network) TransactionManager() driver.TransactionManager {
	return f.transactionManager
}

func (f *network) TransactionService() driver.TransactionService {
	return f.transactionService
}

func (f *network) MetadataService() driver.MetadataService {
	return f.metadataService
}

func (f *network) EnvelopeService() driver.EnvelopeService {
	return f.envelopeService
}

func (f *network) Vault() driver.Vault {
	return f.vault
}

func (f *network) ProcessorManager() driver.ProcessorManager {
	return f.processorManager
}

func (f *network) Committer() driver.Committer {
	return f.committer
}

func (f *network) Finality() driver.Finality {
	return f.finality
}

func (f *network) DeliveryService() driver.DeliveryService {
	return f.deliveryService
}
