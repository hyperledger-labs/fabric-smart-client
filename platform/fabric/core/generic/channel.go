/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"context"
	"fmt"
	"io/ioutil"
	"runtime/debug"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"

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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var (
	waitForEventTimeout = 300 * time.Second
)

// These are function names from Invoke first parameter
const (
	GetBlockByNumber   string = "GetBlockByNumber"
	GetTransactionByID string = "GetTransactionByID"
	GetBlockByTxID     string = "GetBlockByTxID"
)

type channel struct {
	commitSubscriptions sync.Map
	sp                  view2.ServiceProvider
	config              *config2.Config
	network             *network
	name                string
	finality            driver.Finality
	vault               *vault.Vault
	processNamespaces   []string
	externalCommitter   *committer.ExternalCommitter
	envelopeService     driver.EnvelopeService
	transactionService  driver.EndorserTransactionService
	metadataService     driver.MetadataService
	driver.TXIDStore

	// applyLock is used to serialize calls to CommitConfig and bundle update processing.
	applyLock sync.Mutex
	// lock is used to serialize access to resources
	lock sync.RWMutex
	// resources is used to acquire configuration bundle resources.
	resources channelconfig.Resources
}

func newChannel(network *network, name string, quiet bool) (*channel, error) {
	sp := network.sp
	// Vault
	v, txIDStore, err := NewVault(network.config, name, sp)
	if err != nil {
		return nil, err
	}

	// Fabric finality
	fabricFinality, err := finality2.NewFabricFinality(
		name,
		network,
		hash.GetHasher(sp),
		waitForEventTimeout,
	)
	if err != nil {
		return nil, err
	}

	// Committers
	externalCommitter, err := committer.GetExternalCommitter(name, sp, v)
	if err != nil {
		return nil, err
	}

	committerInst, err := committer.New(name, network, fabricFinality, waitForEventTimeout, quiet)
	if err != nil {
		return nil, err
	}

	c := &channel{
		name:               name,
		config:             network.config,
		network:            network,
		vault:              v,
		sp:                 sp,
		externalCommitter:  externalCommitter,
		TXIDStore:          txIDStore,
		envelopeService:    transaction.NewEnvelopeService(sp, network.Name(), name),
		transactionService: transaction.NewEndorseTransactionService(sp, network.Name(), name),
		metadataService:    transaction.NewMetadataService(sp, network.Name(), name),
	}

	// Finality
	fs, err := finality2.NewService(sp, network, name, c)
	if err != nil {
		return nil, err
	}

	c.finality = fs

	if err := c.init(); err != nil {
		return nil, errors.WithMessagef(err, "failed initializing channel [%s]", name)
	}

	// Delivery
	deliveryService, err := delivery2.New(
		network.ctx,
		name,
		sp,
		network,
		func(block *peer.FilteredBlock) (bool, error) {
			c.publishToSubscribers(block)
			if err := committerInst.Commit(block); err != nil {
				switch errors.Cause(err) {
				case committer.ErrQSCCUnreachable:
					return false, delivery2.ErrComm
				default:
					return true, err
				}
			}
			return false, nil
		},
		txIDStore,
		waitForEventTimeout,
	)
	if err != nil {
		return nil, err
	}

	// Start delivery
	deliveryService.Start()

	return c, nil
}

func (c *channel) Name() string {
	return c.name
}

func (c *channel) GetTLSRootCert(endorser view.Identity) ([][]byte, error) {
	return c.network.GetTLSRootCert(endorser)
}

func (c *channel) NewPeerClientForIdentity(peer view.Identity) (peer2.PeerClient, error) {
	addresses, err := view2.GetEndpointService(c.sp).Endpoint(peer)
	if err != nil {
		return nil, err
	}
	tlsRootCerts, err := c.GetTLSRootCert(peer)
	if err != nil {
		return nil, err
	}
	if addresses[view2.ListenPort] == "" {
		return nil, errors.New("peer address must be set")
	}

	clientConfig, override, err := c.GetClientConfig(tlsRootCerts)
	if err != nil {
		return nil, err
	}

	return newPeerClientForClientConfig(addresses[view2.ListenPort], override, *clientConfig)
}

func (c *channel) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.PeerClient, error) {
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

	clientConfig, override, err := c.GetClientConfig(certs)
	if err != nil {
		return nil, err
	}

	if len(cc.ServerNameOverride) != 0 {
		override = cc.ServerNameOverride
	}

	return newPeerClientForClientConfig(
		cc.Address,
		override,
		*clientConfig,
	)
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

	return clientConfig, override, nil
}

func (c *channel) GetTransactionByID(txID string) (driver.ProcessedTransaction, error) {
	raw, err := c.Chaincode("qscc").NewInvocation(GetTransactionByID, c.name, txID).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.Peers()...).Query()
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

func (c *channel) publishToSubscribers(block *peer.FilteredBlock) {
	for _, tx := range block.FilteredTransactions {
		var err error
		if tx.TxValidationCode > 0 {
			err = fmt.Errorf("tx invalid: %s", tx.TxValidationCode.String())
		}
		c.publish(tx.Txid, err)
	}
}

func (c *channel) publish(txID string, err error) {
	logger.Debugf("Publishing commit of %s", txID)
	o, exists := c.commitSubscriptions.Load(txID)
	if !exists {
		logger.Debugf("No one subscribed to %s", txID)
		return
	}

	sub := o.(chan error)

	select {
	case sub <- err:
		return
	default:
	}
}

func (c *channel) Subscribe(txID string) {
	c.subscribeTxCommit(txID)
}

func (c *channel) WaitForSubscription(txID string, ctx context.Context) error {
	o, exists := c.commitSubscriptions.Load(txID)
	logger.Debugf("Publishing commit of %s", txID)
	if !exists {
		debug.PrintStack()
		return errors.Errorf("no prior broadcast detected for %s", txID)
	}

	defer c.commitSubscriptions.Delete(txID)

	sub := o.(chan error)

	select {
	case err := <-sub:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *channel) subscribeTxCommit(txID string) {
	c.commitSubscriptions.LoadOrStore(txID, make(chan error, 1))
}

func (c *channel) GetBlockNumberByTxID(txID string) (uint64, error) {
	res, err := c.Chaincode("qscc").NewInvocation(GetBlockByTxID, c.name, txID).WithSignerIdentity(
		c.network.LocalMembership().DefaultIdentity(),
	).WithEndorsersByConnConfig(c.network.Peers()...).Query()
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
	return nil
}

func newPeerClientForClientConfig(address, override string, clientConfig grpc.ClientConfig) (*common2.PeerClient, error) {
	gClient, err := grpc.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create PeerClient from config")
	}
	pClient := &common2.PeerClient{
		CommonClient: common2.CommonClient{
			Client:  gClient,
			Address: address,
			Sn:      override}}
	return pClient, nil
}

type processedTransaction struct {
	vc  int32
	ue  *transaction.UnpackedEnvelope
	env []byte
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
