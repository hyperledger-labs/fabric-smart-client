/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/finality"
	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
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
	DefaultNumRetries         = 3
	DefaultRetrySleep         = 1 * time.Second
)

type Delivery interface {
	Start(ctx context.Context)
	Stop()
}

type ChannelProvider interface {
	NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error)
}

type RWSetLoaderConstructor = func(network string, channel string, envelopeService driver.EnvelopeService, transactionService driver.EndorserTransactionService, transactionManager driver.TransactionManager, vault *vault.Vault) driver.RWSetLoader
type VaultConstructor = func(sp view2.ServiceProvider, config *config.Config, channel string) (*vault.Vault, TXIDStore, error)

func NewGenericChannelProvider(
	committerProvider CommitterProvider,
	eventsPublisher events.Publisher,
	eventsSubscriber events.Subscriber,
	hasher hash.Hasher,
	viewManager api2.ViewManager,
	kvsStore endpoint.KVS,
) *provider {
	return NewChannelProvider(committerProvider, eventsPublisher, eventsSubscriber, hasher, viewManager, kvsStore, NewRWSetLoader, NewVault)
}

func NewChannelProvider(
	committerProvider CommitterProvider,
	eventsPublisher events.Publisher,
	eventsSubscriber events.Subscriber,
	hasher hash.Hasher,
	viewManager api2.ViewManager,
	kvsStore endpoint.KVS,
	rwSetLoaderConstructor RWSetLoaderConstructor,
	vaultConstructor VaultConstructor,
) *provider {
	return &provider{
		committerProvider:      committerProvider,
		eventsPublisher:        eventsPublisher,
		eventsSubscriber:       eventsSubscriber,
		hasher:                 hasher,
		viewManager:            viewManager,
		kvsStore:               kvsStore,
		rwSetLoaderConstructor: rwSetLoaderConstructor,
		vaultConstructor:       vaultConstructor,
	}
}

type provider struct {
	committerProvider      CommitterProvider
	eventsPublisher        events.Publisher
	eventsSubscriber       events.Subscriber
	rwSetLoaderConstructor RWSetLoaderConstructor
	vaultConstructor       VaultConstructor
	hasher                 hash.Hasher
	viewManager            api2.ViewManager
	kvsStore               endpoint.KVS
}

func (p *provider) NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error) {
	network := nw.(*Network)
	sp := network.SP

	// Vault
	v, txIDStore, err := p.vaultConstructor(sp, network.Config(), name)
	if err != nil {
		return nil, err
	}

	channelConfig, err := getConfig(network.Config(), name)
	if err != nil {
		return nil, err
	}

	// Committers
	externalCommitter, err := committer.GetExternalCommitter(name, sp, v)
	if err != nil {
		return nil, err
	}

	committerInst, err := p.committerProvider.New(network, quiet, channelConfig)
	if err != nil {
		return nil, err
	}

	// Delivery
	deliveryService, err := delivery.New(channelConfig, p.hasher, network, func(block *common.Block) (bool, error) {
		// commit the block, if an error occurs then retry
		err := committerInst.Commit(block)
		return false, err
	}, txIDStore, channelConfig.CommitterWaitForEventTimeout())
	if err != nil {
		return nil, err
	}

	// Finality
	fs, err := finality.NewService(p.viewManager, network, channelConfig, committerInst)
	if err != nil {
		return nil, err
	}

	c := &Channel{
		ChannelName:       name,
		NetworkConfig:     network.Config(),
		ChannelConfig:     channelConfig,
		Network:           network,
		Vault:             v,
		SP:                sp,
		Finality:          fs,
		DeliveryService:   deliveryService,
		ExternalCommitter: externalCommitter,
		TXIDStore:         txIDStore,
		ES:                transaction.NewEnvelopeService(p.kvsStore, network.Name(), name),
		TS:                transaction.NewEndorseTransactionService(p.kvsStore, network.Name(), name),
		MS:                transaction.NewMetadataService(p.kvsStore, network.Name(), name),
		Chaincodes:        map[string]driver.Chaincode{},
		EventsPublisher:   p.eventsPublisher,
		EventsSubscriber:  p.eventsSubscriber,
		Subscribers:       events.NewSubscribers(),
	}
	c.RWSetLoader = p.rwSetLoaderConstructor(network.Name(), name, c.ES, c.TS, network.TransactionManager(), v)
	if err := c.Init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing Channel [%s]", name)
	}

	return c, nil
}

func getConfig(c *config.Config, name string) (*config.Channel, error) {
	// Channel configuration
	channelConfigs, err := c.Channels()
	if err != nil {
		return nil, fmt.Errorf("failed to get Channel config: %w", err)
	}
	for _, config := range channelConfigs {
		if config.Name == name {
			return config, nil
		}
	}
	return &config.Channel{
		Name:       name,
		Default:    false,
		Quiet:      false,
		NumRetries: DefaultNumRetries,
		RetrySleep: DefaultRetrySleep,
		Chaincodes: nil,
	}, nil
}

type Channel struct {
	SP                view2.ServiceProvider
	ChannelConfig     *config.Channel
	NetworkConfig     *config.Config
	Network           *Network
	ChannelName       string
	Finality          driver.Finality
	Vault             *vault.Vault
	ProcessNamespaces []string
	ExternalCommitter *committer.ExternalCommitter
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
	ConnCache common2.CachingEndorserPool

	// events
	Subscribers      *events.Subscribers
	EventsSubscriber events.Subscriber
	EventsPublisher  events.Publisher
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
	override := c.NetworkConfig.TLSServerHostOverride()
	clientConfig := &grpc.ClientConfig{}
	clientConfig.Timeout = c.NetworkConfig.ClientConnTimeout()
	if clientConfig.Timeout == time.Duration(0) {
		clientConfig.Timeout = grpc.DefaultConnectionTimeout
	}

	secOpts := grpc.SecureOptions{
		UseTLS:            UseTLS,
		RequireClientCert: c.NetworkConfig.TLSClientAuthRequired(),
	}
	if UseTLS {
		secOpts.RequireClientCert = false
	}

	if secOpts.RequireClientCert {
		keyPEM, err := os.ReadFile(c.NetworkConfig.TLSClientKeyFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := os.ReadFile(c.NetworkConfig.TLSClientCertFile())
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
		ClientInterval: c.NetworkConfig.KeepAliveClientInterval(),
		ClientTimeout:  c.NetworkConfig.KeepAliveClientTimeout(),
	}

	return clientConfig, override, nil
}

func (c *Channel) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.Chaincode("qscc").NewInvocation(GetTransactionByID, c.ChannelName, txID).WithSignerIdentity(
		c.Network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.Network.PickPeer(driver.PeerForQuery)).Query()
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
	).WithEndorsersByConnConfig(c.Network.PickPeer(driver.PeerForQuery)).Query()
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

func (c *Channel) Config() *config.Channel {
	return c.ChannelConfig
}

func (c *Channel) DefaultSigner() discovery.Signer {
	return c.Network.LocalMembership().DefaultSigningIdentity().Sign
}

// FetchAndStoreEnvelope fetches from the ledger and stores the enveloped correspoding to the passed id
func (c *Channel) FetchAndStoreEnvelope(txID string) error {
	pt, err := c.GetTransactionByID(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	if err := c.EnvelopeService().StoreEnvelope(txID, pt.Envelope()); err != nil {
		return errors.WithMessagef(err, "failed to store fetched envelope for [%s]", txID)
	}
	return nil
}

func (c *Channel) GetRWSetFromEvn(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	return c.RWSetLoader.GetRWSetFromEvn(txID)
}

func (c *Channel) GetRWSetFromETx(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	return c.RWSetLoader.GetRWSetFromETx(txID)
}

func (c *Channel) Init() error {
	if err := c.ReloadConfigTransactions(); err != nil {
		return errors.WithMessagef(err, "failed reloading config transactions")
	}
	c.ConnCache = common2.CachingEndorserPool{
		Cache:       map[string]peer2.Client{},
		ConnCreator: &connCreator{ch: c},
		Signer:      c.DefaultSigner(),
	}
	return nil
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
