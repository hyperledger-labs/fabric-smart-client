/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
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
	WaitForEventTimeout = 300 * time.Second
	FinalityWaitTimeout = 20 * time.Second
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

type Channel struct {
	SP                view2.ServiceProvider
	ChannelConfig     *config2.Channel
	NetworkConfig     *config2.Config
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

func NewChannel(nw driver.FabricNetworkService, name string, quiet bool) (driver.Channel, error) {
	network := nw.(*Network)
	sp := network.SP
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
		FinalityWaitTimeout,
	)
	if err != nil {
		return nil, err
	}

	// Committers
	externalCommitter, err := committer.GetExternalCommitter(name, sp, v)
	if err != nil {
		return nil, err
	}

	publisher, err := events.GetPublisher(network.SP)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get event publisher")
	}

	committerInst, err := committer.New(name, network, fabricFinality, WaitForEventTimeout, quiet, tracing.Get(sp).GetTracer(), publisher)
	if err != nil {
		return nil, err
	}

	// Delivery
	deliveryService, err := delivery2.New(name, sp, network, func(block *common.Block) (bool, error) {
		// commit the block, if an error occurs then retry
		err := committerInst.Commit(block)
		return false, err
	}, txIDStore, WaitForEventTimeout)
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

	// Channel configuration
	channelConfigs, err := network.config.Channels()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get Channel config")
	}
	var channelConfig *config2.Channel
	for _, config := range channelConfigs {
		if config.Name == name {
			channelConfig = config
			break
		}
	}
	if channelConfig != nil {
		channelConfig = &config2.Channel{
			Name:       name,
			Default:    false,
			Quiet:      false,
			NumRetries: DefaultNumRetries,
			RetrySleep: DefaultRetrySleep,
			Chaincodes: nil,
		}
	}
	c := &Channel{
		ChannelName:       name,
		NetworkConfig:     network.config,
		ChannelConfig:     channelConfig,
		Network:           network,
		Vault:             v,
		SP:                sp,
		Finality:          fs,
		DeliveryService:   deliveryService,
		ExternalCommitter: externalCommitter,
		TXIDStore:         txIDStore,
		ES:                transaction.NewEnvelopeService(sp, network.Name(), name),
		TS:                transaction.NewEndorseTransactionService(sp, network.Name(), name),
		MS:                transaction.NewMetadataService(sp, network.Name(), name),
		Chaincodes:        map[string]driver.Chaincode{},
		EventsPublisher:   eventsPublisher,
		EventsSubscriber:  eventsSubscriber,
		Subscribers:       events.NewSubscribers(),
	}
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
		keyPEM, err := ioutil.ReadFile(c.NetworkConfig.TLSClientKeyFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := ioutil.ReadFile(c.NetworkConfig.TLSClientCertFile())
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

func (c *Channel) Config() *config2.Channel {
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
	rawEnv, err := c.EnvelopeService().LoadEnvelope(txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load envelope [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s,%s]", c.Name(), txID)
	env := &common.Envelope{}
	err = proto.Unmarshal(rawEnv, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unmarshalling envelope [%s]", txID)
	}
	logger.Debugf("unpack envelope [%s,%s]", c.Name(), txID)
	upe, err := rwset.UnpackEnvelope(c.Network.Name(), env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unpacking envelope [%s]", txID)
	}
	logger.Debugf("retrieve rws [%s,%s]", c.Name(), txID)

	rws, err := c.GetRWSet(txID, upe.Results)
	if err != nil {
		return nil, nil, err
	}

	return rws, upe, nil
}

func (c *Channel) GetRWSetFromETx(txID string) (driver.RWSet, driver.ProcessTransaction, error) {
	raw, err := c.TransactionService().LoadTransaction(txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load etx [%s]", txID)
	}
	tx, err := c.Network.TransactionManager().NewTransactionFromBytes(c.Name(), raw)
	if err != nil {
		return nil, nil, err
	}
	rws, err := tx.GetRWSet()
	if err != nil {
		return nil, nil, err
	}

	return rws, tx, nil
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
	ch *Channel
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
