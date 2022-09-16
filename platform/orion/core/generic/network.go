/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/committer"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
)

var (
	logger              = flogging.MustGetLogger("orion-sdk.core")
	waitForEventTimeout = 300 * time.Second
)

type network struct {
	sp  view2.ServiceProvider
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
}

func NewDB(ctx context.Context, sp view2.ServiceProvider, config *config2.Config, name string) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		sp:     sp,
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
	n.metadataService = transaction.NewMetadataService(sp, name)
	n.envelopeService = transaction.NewEnvelopeService(sp, name)
	n.transactionManager = transaction.NewManager(sp, n.sessionManager)
	n.transactionService = transaction.NewEndorseTransactionService(sp, name)
	n.processorManager = rwset.NewProcessorManager(n.sp, n, nil)

	return n, nil
}

func NewNetwork(ctx context.Context, sp view2.ServiceProvider, config *config2.Config, name string) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		sp:     sp,
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
	n.metadataService = transaction.NewMetadataService(sp, name)
	n.envelopeService = transaction.NewEnvelopeService(sp, name)
	n.transactionManager = transaction.NewManager(sp, n.sessionManager)
	n.transactionService = transaction.NewEndorseTransactionService(sp, name)
	n.vault, err = NewVault(sp, n.config, name)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create vault")
	}
	n.processorManager = rwset.NewProcessorManager(n.sp, n, nil)

	// events
	eventsPublisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event publisher")
	}
	eventsSubscriber, err := events.GetSubscriber(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event subscriber")
	}

	committer, err := committer.New(
		name,
		n.processorManager,
		n.vault,
		nil,
		waitForEventTimeout,
		false,
		eventsPublisher,
		eventsSubscriber,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create committer")
	}
	n.committer = committer

	finality, err := finality.NewService(committer)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create finality service")
	}
	n.finality = finality

	deliveryService, err := delivery2.New(
		n.ctx,
		sp,
		n,
		func(block *types.AugmentedBlockHeader) (bool, error) {
			if err := committer.Commit(block); err != nil {
				return true, err
			}
			return false, nil
		},
		n.vault,
		waitForEventTimeout,
	)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to create delivery service")
	}
	deliveryService.Start()

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
