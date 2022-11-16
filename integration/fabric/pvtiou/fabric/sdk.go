/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"context"
	"os"

	_ "github.com/go-kivik/couchdb/v3"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/pvtiou/fabric/couchdb"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common/runner"
	fabric2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/pvt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
	"github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/tedsuo/ifrit"
)

var logger = flogging.MustGetLogger("fabric-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)
	RegisterService(service interface{}) error
}

type Startable interface {
	Start(ctx context.Context) error
	Stop() error
}

type SDK struct {
	registry             Registry
	fnsProvider          Startable
	couchProcess         ifrit.Process
	couchdbSyncProcessor *couchdb.SyncProcessor
	couchDB              *runner.CouchDB
}

func NewSDK(registry Registry) *SDK {
	return &SDK{registry: registry}
}

func (p *SDK) Install() error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping")
		return nil
	}
	logger.Infof("Fabric platform enabled, installing...")

	cryptoProvider := crypto.NewProvider()
	assert.NoError(p.registry.RegisterService(cryptoProvider))

	logger.Infof("Set Fabric Network Service Provider")
	fnsConfig, err := core.NewConfig(view2.GetConfigService(p.registry))
	assert.NoError(err, "failed parsing configuration")
	p.fnsProvider, err = core.NewFabricNetworkServiceProvider(p.registry, fnsConfig)
	assert.NoError(err, "failed instantiating fabric network service provider")
	assert.NoError(p.registry.RegisterService(p.fnsProvider))
	assert.NoError(p.registry.RegisterService(fabric2.NewNetworkServiceProvider(p.registry)))

	// Register processors
	p.couchdbSyncProcessor = &couchdb.SyncProcessor{}

	names := fabric2.GetFabricNetworkNames(p.registry)
	if len(names) == 0 {
		return errors.New("no fabric network names found")
	}
	for _, name := range names {
		fns := fabric2.GetFabricNetworkService(p.registry, name)
		assert.NotNil(fns, "no fabric network service found for [%s]", name)
		assert.NoError(
			fns.ProcessorManager().SetDefaultProcessor(
				state.NewRWSetProcessor(fabric2.GetDefaultFNS(p.registry)),
			),
			"failed setting state processor for fabric network [%s]", name,
		)

		networkConfig, err := fnsConfig.Network(name)
		assert.NoError(err, "failed to get network config for [%s]", name)
		assert.NoError(pvt.Install(fns, networkConfig), "failed to install Private Transactions processors")
		assert.NoError(
			fns.ProcessorManager().AddProcessor("iou", p.couchdbSyncProcessor),
			"failed to register couchdb sync processor for fabric network [%s]", name,
		)
	}

	assert.NotNil(fabric2.GetDefaultFNS(p.registry), "default fabric network service not found")

	// TODO: remove this
	assert.NoError(p.registry.RegisterService(tracker.NewTracker()))
	// TODO: change this
	assert.NoError(p.registry.RegisterService(vault.NewService(p.registry)))

	// weaver provider
	assert.NoError(p.registry.RegisterService(weaver.NewProvider()))

	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	// run couch db instance
	gomega.RegisterFailHandler(func(message string, callerSkip ...int) {
		panic(message)
	})
	p.couchDB = &runner.CouchDB{}
	p.couchProcess = ifrit.Invoke(p.couchDB)
	gomega.Eventually(p.couchProcess.Ready(), runner.DefaultStartTimeout).Should(gomega.BeClosed())
	gomega.Consistently(p.couchProcess.Wait()).ShouldNot(gomega.Receive())
	config := couchdb.Config{}
	config.Address = p.couchDB.Address()
	config.User = runner.CouchDBUsername
	config.Password = runner.CouchDBPassword
	manager := couchdb.NewManager(config)
	assert.NoError(p.registry.RegisterService(manager), "failed to register couchdb manager")
	var err error
	p.couchdbSyncProcessor.Client, err = manager.Client()
	assert.NoError(err, "failed to get couchdb client to [%s]", p.couchDB.Address())

	return nil
}

func (p *SDK) PostStart(ctx context.Context) error {
	if !view2.GetConfigService(p.registry).GetBool("fabric.enabled") {
		logger.Infof("Fabric platform not enabled, skipping start")
		return nil
	}

	// start the network service
	if err := p.fnsProvider.Start(ctx); err != nil {
		return errors.WithMessagef(err, "failed starting fabric network service provider")
	}

	go func() {
		<-ctx.Done()
		if err := p.fnsProvider.Stop(); err != nil {
			logger.Errorf("failed stopping fabric network service provider [%s]", err)
		}

		p.couchProcess.Signal(os.Kill)
		p.couchDB.Stop()
	}()

	return nil
}
