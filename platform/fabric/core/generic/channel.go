/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"io/ioutil"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	discovery "github.com/hyperledger/fabric/discovery/client"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

var (
	waitForEventTimeout = 300 * time.Second
	finalityWaitTimeout = 20 * time.Second
)

// These are function names from Invoke first parameter
const (
	GetBlockByNumber   string = "GetBlockByNumber"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

type channel struct {
	sp                 view2.ServiceProvider
	config             *config2.Config
	network            *network
	name               string
	finality           driver.Finality
	vault              *vault.Vault
	processNamespaces  []string
	externalCommitter  *committer.ExternalCommitter
	envelopeService    driver.EnvelopeService
	transactionService driver.EndorserTransactionService
	metadataService    driver.MetadataService
	eventsSubscriber   events.Subscriber
	eventsPublisher    events.Publisher
	driver.TXIDStore

	// applyLock is used to serialize calls to CommitConfig and bundle update processing.
	applyLock sync.Mutex
	// lock is used to serialize access to resources
	lock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	resources channelconfig.Resources

	chaincodesLock sync.RWMutex
	chaincodes     map[string]driver.Chaincode

	connCache common2.CachingEndorserPool

	// subscribers
	subscribers *events.Subscribers
}

func newChannel(network *network, name string, quiet bool) (*channel, error) {
	sp := network.sp
	// Vault
	v, txIDStore, err := NewVault(sp, network.config, name)
	if err != nil {
		return nil, err
	}

	// Fabric finality
	fabricFinality, err := finality2.NewFabricFinality(
		name,
		network,
		hash.GetHasher(sp),
		finalityWaitTimeout,
	)
	if err != nil {
		return nil, err
	}

	// Committers
	externalCommitter, err := committer.GetExternalCommitter(name, sp, v)
	if err != nil {
		return nil, err
	}

	committerInst, err := committer.New(name, network, fabricFinality, waitForEventTimeout, quiet, tracing.Get(sp))
	if err != nil {
		return nil, err
	}

	// Delivery
	deliveryService, err := delivery2.New(
		network.ctx,
		name,
		sp,
		network,
		func(block *common.Block) (bool, error) {
			if err := committerInst.Commit(block); err != nil {
				return true, err
			}
			return false, nil
		},
		txIDStore,
		waitForEventTimeout,
	)
	if err != nil {
		return nil, err
	}

	// Finality
	fs, err := finality2.NewService(sp, network, name, committerInst)
	if err != nil {
		return nil, err
	}
	// Events
	eventsPublisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event publisher")
	}
	eventsSubscriber, err := events.GetSubscriber(sp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get event subscriber")
	}

	c := &channel{
		name:               name,
		config:             network.config,
		network:            network,
		vault:              v,
		sp:                 sp,
		finality:           fs,
		externalCommitter:  externalCommitter,
		TXIDStore:          txIDStore,
		envelopeService:    transaction.NewEnvelopeService(sp, network.Name(), name),
		transactionService: transaction.NewEndorseTransactionService(sp, network.Name(), name),
		metadataService:    transaction.NewMetadataService(sp, network.Name(), name),
		chaincodes:         map[string]driver.Chaincode{},
		eventsPublisher:    eventsPublisher,
		eventsSubscriber:   eventsSubscriber,
		subscribers:        events.NewSubscribers(),
	}
	if err := c.init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing channel [%s]", name)
	}

	// Start delivery
	if network.Config().IsDeliveryEnabled() {
		logger.Debugf("Starting delivery for channel [%s]", name)
		deliveryService.Start()
	} else {
		logger.Debugf("Delivery is disabled for channel [%s]", name)
	}

	return c, nil
}

func (c *channel) Name() string {
	return c.name
}

func (c *channel) GetTLSRootCert(endorser view.Identity) ([][]byte, error) {
	return c.network.GetTLSRootCert(endorser)
}

func (c *channel) NewPeerClientForIdentity(peer view.Identity) (peer2.Client, error) {
	logger.Debugf("NewPeerClientForIdentity [%s]", peer)
	return c.connCache.NewPeerClientForIdentity(peer)
}

func (c *channel) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("NewPeerClientForAddress [%v]", cc)
	return c.connCache.NewPeerClientForAddress(cc)
}

func (c *channel) IsValid(identity view.Identity) error {
	id, err := c.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *channel) GetVerifier(identity view.Identity) (api2.Verifier, error) {
	id, err := c.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}
	return id, nil
}

func (c *channel) GetClientConfig(tlsRootCerts [][]byte) (*grpc.ClientConfig, string, error) {
	override := c.config.TLSServerHostOverride()
	clientConfig := &grpc.ClientConfig{}
	clientConfig.Timeout = c.config.ClientConnTimeout()
	if clientConfig.Timeout == time.Duration(0) {
		clientConfig.Timeout = grpc.DefaultConnectionTimeout
	}

	secOpts := grpc.SecureOptions{
		UseTLS:            c.config.TLSEnabled(),
		RequireClientCert: c.config.TLSClientAuthRequired(),
	}

	if secOpts.RequireClientCert {
		keyPEM, err := ioutil.ReadFile(c.config.TLSClientKeyFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := ioutil.ReadFile(c.config.TLSClientCertFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientCert.file")
		}
		secOpts.Certificate = certPEM
	}
	clientConfig.SecOpts = secOpts

	if clientConfig.SecOpts.UseTLS {
		if len(tlsRootCerts) == 0 {
			return nil, "", errors.New("tls root cert file must be set")
		}
		clientConfig.SecOpts.ServerRootCAs = tlsRootCerts
	}

	clientConfig.KaOpts = grpc.KeepaliveOptions{
		ClientInterval:    c.config.KeepAliveClientInterval(),
		ClientTimeout:     c.config.KeepAliveClientTimeout(),
		ServerInterval:    c.config.KeepAliveServerInterval(),
		ServerTimeout:     c.config.KeepAliveServerTimeout(),
		ServerMinInterval: c.config.KeepAliveServerMinInterval(),
	}

	return clientConfig, override, nil
}

func (c *channel) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.Chaincode("qscc").NewInvocation(GetTransactionByID, c.name, txID).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.PickPeer()).Query()
	if err != nil {
		return nil, err
	}

	logger.Debugf("got transaction by id [%s] of len [%d]", txID, len(raw))

	pt := &peer.ProcessedTransaction{}
	err = proto.Unmarshal(raw, pt)
	if err != nil {
		return nil, err
	}
	return newProcessedTransaction(pt)
}

func (c *channel) GetBlockNumberByTxID(txID string) (uint64, error) {
	res, err := c.Chaincode("qscc").NewInvocation(GetBlockByTxID, c.name, txID).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.PickPeer()).Query()
	if err != nil {
		return 0, err
	}

	block := &common.Block{}
	err = proto.Unmarshal(res, block)
	if err != nil {
		return 0, err
	}
	return block.Header.Number, nil
}

func (c *channel) Close() error {
	return c.vault.Close()
}

func (c *channel) init() error {
	if err := c.ReloadConfigTransactions(); err != nil {
		return errors.WithMessagef(err, "failed reloading config transactions")
	}
	c.connCache = common2.CachingEndorserPool{
		Cache:       map[string]peer2.Client{},
		ConnCreator: &connCreator{ch: c},
		Signer:      c.DefaultSigner(),
	}
	return nil
}

func (c *channel) DefaultSigner() discovery.Signer {
	return c.network.LocalMembership().DefaultSigningIdentity().Sign
}

func newPeerClientForClientConfig(signer discovery.Signer, address, override string, clientConfig grpc.ClientConfig) (*common2.PeerClient, error) {
	gClient, err := grpc.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create Client from config")
	}
	pClient := &common2.PeerClient{
		Signer: signer,
		CommonClient: common2.CommonClient{
			Client:  gClient,
			Address: address,
			Sn:      override,
		},
	}
	return pClient, nil
}

type processedTransaction struct {
	vc  int32
	ue  *transaction.UnpackedEnvelope
	env []byte
}

func newProcessedTransactionFromEnvelope(env *common.Envelope) (*processedTransaction, error) {
	ue, err := transaction.UnpackEnvelope(env)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{ue: ue}, nil
}

func newProcessedTransactionFromEnvelopeRaw(env []byte) (*processedTransaction, error) {
	ue, err := transaction.UnpackEnvelopeFromBytes(env)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{ue: ue, env: env}, nil
}

func newProcessedTransaction(pt *peer.ProcessedTransaction) (*processedTransaction, error) {
	ue, err := transaction.UnpackEnvelope(pt.TransactionEnvelope)
	if err != nil {
		return nil, err
	}
	env, err := protoutil.Marshal(pt.TransactionEnvelope)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{vc: pt.ValidationCode, ue: ue, env: env}, nil
}

func (p *processedTransaction) TxID() string {
	return p.ue.TxID
}

func (p *processedTransaction) Results() []byte {
	return p.ue.Results
}

func (p *processedTransaction) IsValid() bool {
	return p.vc == int32(peer.TxValidationCode_VALID)
}

func (p *processedTransaction) Envelope() []byte {
	return p.env
}

func (p *processedTransaction) ValidationCode() int32 {
	return p.vc
}

type connCreator struct {
	ch *channel
}

func (c *connCreator) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("Creating new peer client for address [%s]", cc.Address)
	var certs [][]byte
	if cc.TLSEnabled {
		switch {
		case len(cc.TLSRootCertFile) != 0:
			logger.Debugf("Loading TLSRootCert from file [%s]", cc.TLSRootCertFile)
			caPEM, err := ioutil.ReadFile(cc.TLSRootCertFile)
			if err != nil {
				logger.Error("unable to load TLS cert from %s", cc.TLSRootCertFile)
				return nil, errors.WithMessagef(err, "unable to load TLS cert from %s", cc.TLSRootCertFile)
			}
			certs = append(certs, caPEM)
		case len(cc.TLSRootCertBytes) != 0:
			logger.Debugf("Loading TLSRootCert from passed bytes [%s[", cc.TLSRootCertBytes)
			certs = cc.TLSRootCertBytes
		default:
			return nil, errors.New("missing TLSRootCertFile in client config")
		}
	}

	clientConfig, override, err := c.ch.GetClientConfig(certs)
	if err != nil {
		return nil, err
	}

	if len(cc.ServerNameOverride) != 0 {
		override = cc.ServerNameOverride
	}

	return newPeerClientForClientConfig(
		c.ch.DefaultSigner(),
		cc.Address,
		override,
		*clientConfig,
	)
}

func (c *connCreator) NewPeerClientForIdentity(peer view.Identity) (peer2.Client, error) {
	logger.Debugf("Creating new peer client for [%s]", peer)
	addresses, err := view2.GetEndpointService(c.ch.sp).Endpoint(peer)
	if err != nil {
		return nil, err
	}
	tlsRootCerts, err := c.ch.GetTLSRootCert(peer)
	if err != nil {
		return nil, err
	}
	if addresses[view2.ListenPort] == "" {
		return nil, errors.New("peer address must be set")
	}

	clientConfig, override, err := c.ch.GetClientConfig(tlsRootCerts)
	if err != nil {
		return nil, err
	}

	return newPeerClientForClientConfig(
		c.ch.DefaultSigner(),
		addresses[view2.ListenPort],
		override,
		*clientConfig,
	)
}
