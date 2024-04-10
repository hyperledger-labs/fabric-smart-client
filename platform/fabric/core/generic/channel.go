/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	delivery2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	finality2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
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

// These are function names from Invoke first parameter
const (
	GetBlockByNumber   string = "GetBlockByNumber"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

type Delivery interface {
	Start(ctx context.Context)
	Stop()
}

type Channel struct {
	SP                view2.ServiceProvider
	ChannelConfig     driver.ChannelConfig
	ConfigService     driver.ConfigService
	Network           *Network
	ChannelName       string
	Finality          driver.Finality
	Vault             *vault.Vault
	ProcessNamespaces []string
	StatusReporters   []driver.StatusReporter
	ES                driver.EnvelopeService
	TS                driver.EndorserTransactionService
	MS                driver.MetadataService
	DeliveryService   Delivery
	driver.TXIDStore
	RWSetLoader driver.RWSetLoader

	// ResourcesApplyLock is used to serialize calls to CommitConfig and bundle update processing.
	ResourcesApplyLock sync.Mutex
	// ResourcesLock is used to serialize access to resources
	ResourcesLock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	ChannelResources channelconfig.Resources

	// chaincodes
	ChaincodesLock sync.RWMutex
	Chaincodes     map[string]driver.Chaincode

	// connection pool
	ConnCache peer2.CachingEndorserPool

	// events
	Subscribers      *events.Subscribers
	EventsSubscriber events.Subscriber
	EventsPublisher  events.Publisher
}

func NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error) {
	network := nw.(*Network)
	sp := network.SP

	// Channel configuration
	channelConfigs := network.ConfigService().Channels()
	var channelConfig driver.ChannelConfig
	for _, config := range channelConfigs {
		if config.ID() == name {
			channelConfig = config
			break
		}
	}
	if channelConfig == nil {
		channelConfig = network.ConfigService().NewDefaultChannelConfig(name)
	}

	// Vault
	v, txIDStore, err := NewVault(sp, network.configService, name)
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

	kvsService := kvs.GetService(sp)

	c := &Channel{
		ChannelName:      name,
		ConfigService:    network.configService,
		ChannelConfig:    channelConfig,
		Network:          network,
		Vault:            v,
		SP:               sp,
		TXIDStore:        txIDStore,
		ES:               transaction.NewEnvelopeService(kvsService, network.Name(), name),
		TS:               transaction.NewEndorseTransactionService(kvsService, network.Name(), name),
		MS:               transaction.NewMetadataService(kvsService, network.Name(), name),
		Chaincodes:       map[string]driver.Chaincode{},
		EventsPublisher:  eventsPublisher,
		EventsSubscriber: eventsSubscriber,
		Subscribers:      events.NewSubscribers(),
	}

	// Fabric finality
	fabricFinality, err := finality2.NewFabricFinality(
		name,
		network.ConfigService(),
		c,
		network.LocalMembership().DefaultSigningIdentity(),
		hash.GetHasher(sp),
		channelConfig.FinalityWaitTimeout(),
	)
	if err != nil {
		return nil, err
	}

	// Committers
	publisher, err := events.GetPublisher(network.SP)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get event publisher")
	}

	committerInst, err := committer.New(
		channelConfig,
		network,
		fabricFinality,
		channelConfig.CommitterWaitForEventTimeout(),
		quiet,
		tracing.Get(sp).GetTracer(),
		publisher,
	)
	if err != nil {
		return nil, err
	}

	// Finality
	fs, err := finality2.NewService(committerInst)
	if err != nil {
		return nil, err
	}
	c.Finality = fs

	// Delivery
	deliveryService, err := delivery2.New(
		network.Name(),
		channelConfig,
		hash.GetHasher(sp),
		network.LocalMembership(),
		network.ConfigService(),
		c,
		c,
		func(block *common.Block) (bool, error) {
			// commit the block, if an error occurs then retry
			err := committerInst.Commit(block)
			return false, err
		},
		txIDStore,
		channelConfig.CommitterWaitForEventTimeout(),
	)
	if err != nil {
		return nil, err
	}
	c.DeliveryService = deliveryService

	c.RWSetLoader = NewRWSetLoader(
		network.Name(), name,
		c.ES, c.TS, network.TransactionManager(),
		v,
	)
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", name)
	}

	return c, nil
}

func (c *Channel) Name() string {
	return c.ChannelName
}

func (c *Channel) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("NewPeerClientForAddress [%v]", cc)
	return c.ConnCache.NewPeerClientForAddress(cc)
}

func (c *Channel) IsValid(identity view.Identity) error {
	id, err := c.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *Channel) GetVerifier(identity view.Identity) (api2.Verifier, error) {
	id, err := c.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}
	return id, nil
}

func (c *Channel) GetClientConfig(tlsRootCerts [][]byte, UseTLS bool) (*grpc.ClientConfig, string, error) {
	override := c.ConfigService.TLSServerHostOverride()
	clientConfig := &grpc.ClientConfig{}
	clientConfig.Timeout = c.ConfigService.ClientConnTimeout()
	if clientConfig.Timeout == time.Duration(0) {
		clientConfig.Timeout = grpc.DefaultConnectionTimeout
	}

	secOpts := grpc.SecureOptions{
		UseTLS:            UseTLS,
		RequireClientCert: c.ConfigService.TLSClientAuthRequired(),
	}
	if UseTLS {
		secOpts.RequireClientCert = false
	}

	if secOpts.RequireClientCert {
		keyPEM, err := os.ReadFile(c.ConfigService.TLSClientKeyFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := os.ReadFile(c.ConfigService.TLSClientCertFile())
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
		ClientInterval: c.ConfigService.KeepAliveClientInterval(),
		ClientTimeout:  c.ConfigService.KeepAliveClientTimeout(),
	}

	return clientConfig, override, nil
}

func (c *Channel) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.Chaincode("qscc").NewInvocation(GetTransactionByID, c.ChannelName, txID).WithSignerIdentity(
		c.Network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.Network.ConfigService().PickPeer(driver.PeerForQuery)).Query()
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

func (c *Channel) GetBlockNumberByTxID(txID string) (uint64, error) {
	res, err := c.Chaincode("qscc").NewInvocation(GetBlockByTxID, c.ChannelName, txID).WithSignerIdentity(
		c.Network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.Network.ConfigService().PickPeer(driver.PeerForQuery)).Query()
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

func (c *Channel) Close() error {
	c.DeliveryService.Stop()
	return c.Vault.Close()
}

func (c *Channel) Config() driver.ChannelConfig {
	return c.ChannelConfig
}

func (c *Channel) DefaultSigner() discovery.Signer {
	return c.Network.LocalMembership().DefaultSigningIdentity().Sign
}

// FetchEnvelope fetches from the ledger and stores the enveloped correspoding to the passed id
func (c *Channel) FetchEnvelope(txID string) ([]byte, error) {
	pt, err := c.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return nil, errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	return pt.Envelope(), nil
}

func (c *Channel) GetRWSetFromEvn(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	return c.RWSetLoader.GetRWSetFromEvn(txID)
}

func (c *Channel) GetRWSetFromETx(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	return c.RWSetLoader.GetRWSetFromETx(txID)
}

func (c *Channel) GetInspectingRWSetFromEvn(txID string, envelopeRaw []byte) (driver.RWSet, driver.ProcessTransaction, error) {
	return c.RWSetLoader.GetInspectingRWSetFromEvn(txID, envelopeRaw)
}

func (c *Channel) Init() error {
	if err := c.ReloadConfigTransactions(); err != nil {
		return errors.WithMessagef(err, "failed reloading config transactions")
	}
	c.ConnCache = peer2.CachingEndorserPool{
		Cache:       map[string]peer2.Client{},
		ConnCreator: &connCreator{ch: c},
		Signer:      c.DefaultSigner(),
	}
	return nil
}

func newPeerClientForClientConfig(signer discovery.Signer, address, override string, clientConfig grpc.ClientConfig) (*peer2.PeerClient, error) {
	gClient, err := grpc.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create Client from config")
	}
	pClient := &peer2.PeerClient{
		Signer: signer,
		GRPCClient: peer2.GRPCClient{
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

func newProcessedTransactionFromEnvelope(env *common.Envelope) (*processedTransaction, int32, error) {
	ue, headerType, err := transaction.UnpackEnvelope(env)
	if err != nil {
		return nil, headerType, err
	}
	return &processedTransaction{ue: ue}, headerType, nil
}

func newProcessedTransactionFromEnvelopeRaw(env []byte) (*processedTransaction, error) {
	ue, _, err := transaction.UnpackEnvelopeFromBytes(env)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{ue: ue, env: env}, nil
}

func newProcessedTransaction(pt *peer.ProcessedTransaction) (*processedTransaction, error) {
	ue, _, err := transaction.UnpackEnvelope(pt.TransactionEnvelope)
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
	ch *Channel
}

func (c *connCreator) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("Creating new peer client for address [%s]", cc.Address)
	var certs [][]byte
	if cc.TLSEnabled {
		switch {
		case len(cc.TLSRootCertFile) != 0:
			logger.Debugf("Loading TLSRootCert from file [%s]", cc.TLSRootCertFile)
			caPEM, err := os.ReadFile(cc.TLSRootCertFile)
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

	clientConfig, override, err := c.ch.GetClientConfig(certs, cc.TLSEnabled)
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
