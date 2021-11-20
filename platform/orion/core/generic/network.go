/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"

	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

var logger = flogging.MustGetLogger("orion-sdk.core")

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
	txIDStore          *vault.TXIDStore
	vault              *Vault
	processorManager   driver.ProcessorManager
}

func NewNetwork(
	ctx context.Context,
	sp view2.ServiceProvider,
	config *config2.Config,
	name string,
) (*network, error) {
	// Load configuration
	n := &network{
		ctx:    ctx,
		sp:     sp,
		name:   name,
		config: config,
	}
	ids, err := config.Identities()
	if err != nil {
		return nil, err
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
	n.transactionManager = transaction.NewManager(sp)
	n.vault, err = NewVault(n.config, name, sp)
	if err != nil {
		return nil, err
	}
	n.processorManager = rwset.NewProcessorManager(n.sp, n, nil)

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
