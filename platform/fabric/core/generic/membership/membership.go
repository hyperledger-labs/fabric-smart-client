/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"context"
	"slices"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// configuration is the channel configuration in force together with the
// sequence number of the configuration it came from. The two are held as a
// single value so that a reader can never observe a new configuration paired
// with the sequence of the one it replaced.
type configuration struct {
	channelConfig *channelconfig.ChannelConfig
	sequence      uint64
}

// Service answers membership questions about a channel from its current
// configuration. The configuration is not available when the service is built;
// it arrives with the first configuration block, via [Service.Update]. Until one
// is in force every accessor reports [driver.ErrNotInitialized], or, once a
// block has arrived and been refused, [driver.ErrConfigRejected] —
// see [Service.WithConfigWait] for a view that waits for the first one instead.
type Service struct {
	// config holds the channel configuration once it has been loaded. Reading it
	// goes through the holder, which cannot hand out a configuration that is not
	// there.
	config *deferred.Holder[*configuration]

	channelName string

	// configWait bounds how long the reading accessors wait for a configuration
	// to arrive before reporting [driver.ErrNotInitialized]. Zero (the default)
	// keeps the original non-blocking behavior. Set via [Service.WithConfigWait].
	configWait time.Duration
}

func NewService(channelName string) *Service {
	return &Service{
		config:      deferred.NewHolder[*configuration]("channel [" + channelName + "] configuration"),
		channelName: channelName,
	}
}

// WithConfigWait returns a copy of the service whose reading accessors wait up
// to d for a channel configuration to arrive instead of reporting
// [driver.ErrNotInitialized] straight away. A node can start and serve requests
// before its configuration block is delivered, and a caller that cannot answer
// without the configuration would otherwise fail for that whole window.
//
// The wait applies to every accessor that reads the configuration, and to the
// [driver.MSPManager] returned by [Service.MSPManager]. [Service.Update] never
// waits: it is the call that ends the wait.
//
// A refused configuration is reported as soon as it is refused, without waiting
// out d: waiting cannot turn [driver.ErrConfigRejected] into an answer.
//
// The receiver is left untouched, so the committer — which installs the
// configuration and must never wait on it — can hand out a waiting view without
// becoming one.
func (c *Service) WithConfigWait(d time.Duration) *Service {
	cp := *c
	cp.configWait = d
	return &cp
}

// resolve returns the configuration in force, waiting for one to arrive if this
// service was built with [Service.WithConfigWait].
func (c *Service) resolve() (*configuration, error) {
	if c.configWait <= 0 {
		return c.config.Get()
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.configWait)
	defer cancel()
	return c.config.WaitForValue(ctx)
}

// Update installs the channel configuration carried by env. The previously held
// configuration is kept if env cannot be parsed.
func (c *Service) Update(env *cb.Envelope) error {
	return c.config.Update(func(*configuration, bool) (*configuration, error) {
		return parseConfig(env)
	})
}

func parseConfig(env *cb.Envelope) (*configuration, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get payload from config transaction")
	}

	cenv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	if cenv.Config == nil {
		return nil, errors.New("config envelope carries no config")
	}

	cc, err := channelconfig.NewChannelConfig(cenv.Config.ChannelGroup, factory.GetDefault())
	if err != nil {
		return nil, err
	}

	return &configuration{channelConfig: cc, sequence: cenv.Config.Sequence}, nil
}

// ConfigSequence returns the sequence number of the channel configuration
// currently in force. It is 0 for a channel's genesis configuration and
// increases by one for every configuration update this node has applied, so a
// caller can tell whether a configuration change has reached this node yet. A
// channel with no configuration in force reports driver.ErrNotInitialized or
// driver.ErrConfigRejected instead.
func (c *Service) ConfigSequence() (uint64, error) {
	res, err := c.resolve()
	if err != nil {
		return 0, err
	}

	return res.sequence, nil
}

func (c *Service) IsValid(identity view.Identity) error {
	res, err := c.resolve()
	if err != nil {
		return err
	}

	id, err := res.channelConfig.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (c *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	res, err := c.resolve()
	if err != nil {
		return nil, err
	}

	id, err := res.channelConfig.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration. An empty result means the channel has no organizations; a
// channel with no configuration in force reports driver.ErrNotInitialized or
// driver.ErrConfigRejected instead.
func (c *Service) GetMSPIDs() ([]string, error) {
	res, err := c.resolve()
	if err != nil {
		return nil, err
	}

	var mspIDs []string
	if ac := res.channelConfig.ApplicationConfig(); ac != nil {
		for _, org := range ac.Organizations() {
			mspIDs = append(mspIDs, org.MSPID())
		}
	}

	return mspIDs, nil
}

// IsIdemixMSP reports whether the MSP identified by mspID is of type Idemix.
// A false result means the channel has such an MSP and it is not Idemix; a
// channel with no configuration in force reports driver.ErrNotInitialized or
// driver.ErrConfigRejected instead, so a caller cannot mistake an absent
// configuration for a definitive answer.
func (c *Service) IsIdemixMSP(mspID string) (bool, error) {
	res, err := c.resolve()
	if err != nil {
		return false, err
	}

	ac := res.channelConfig.ApplicationConfig()
	if ac == nil {
		return false, nil
	}

	for _, org := range ac.Organizations() {
		if org.MSPID() == mspID {
			return org.MSP().GetType() == msp.IDEMIX, nil
		}
	}

	return false, nil
}

func (c *Service) OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	res, err := c.resolve()
	if err != nil {
		return "", nil, err
	}

	oc := res.channelConfig.OrdererConfig()
	if oc == nil {
		return "", nil, errors.Errorf("orderer config does not exist for channel [%s]", c.channelName)
	}

	// The network's configured client TLS. Discovered trust anchors augment its pool rather
	// than replacing it: a bootstrap or development setup needs an anchor before the first
	// configuration block has been fetched, and the file cannot remove what the channel
	// supplies.
	networkTLS := cs.NetworkClientTLS()
	connectionTimeout := cs.ClientConnTimeout()

	var newOrderers []*grpc.ConnectionConfig
	orgs := oc.Organizations()
	for _, org := range orgs {
		m := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, m.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, m.GetTLSIntermediateCerts()...)
		for _, endpoint := range org.Endpoints() {
			if len(endpoint) == 0 {
				continue
			}
			endpointTLS := networkTLS
			endpointTLS.ServerRootCAs = append(
				slices.Clone(networkTLS.ServerRootCAs), tlsRootCerts...)
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           endpoint,
				ConnectionTimeout: connectionTimeout,
				TLS:               endpointTLS,
			})
		}
	}

	return oc.ConsensusType(), newOrderers, nil
}

// TLSRootCertsByMSPID returns the TLS root and intermediate certificates, in
// that order, of the application organization with the given MSP ID, taken from
// the channel configuration in force. They are the trust anchor for dialing a
// peer of that organization, so a caller must not accept certificates for the
// same purpose from any less trusted source.
//
// It reports [driver.ErrNotInitialized] while no configuration has arrived and
// [driver.ErrConfigRejected] once one has arrived and been refused, and fails if
// no application organization in the channel has this MSP ID. On a Service built
// with [Service.WithConfigWait] it waits up to that budget for a configuration
// to arrive before reporting [driver.ErrNotInitialized].
func (c *Service) TLSRootCertsByMSPID(mspID string) ([][]byte, error) {
	res, err := c.resolve()
	if err != nil {
		return nil, err
	}

	ac := res.channelConfig.ApplicationConfig()
	if ac == nil {
		return nil, errors.Errorf("application config does not exist for channel [%s]", c.channelName)
	}

	return tlsRootCertsByMSPID(ac, mspID, c.channelName)
}

// tlsRootCertsByMSPID finds the application organization with the given MSP ID
// and returns its TLS root certificates followed by its intermediate ones. It is
// keyed by MSP ID, not by the organization name [channelconfig.Application]
// keys its map on.
func tlsRootCertsByMSPID(ac channelconfig.Application, mspID, channelName string) ([][]byte, error) {
	for _, org := range ac.Organizations() {
		if org.MSPID() != mspID {
			continue
		}

		m := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, m.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, m.GetTLSIntermediateCerts()...)
		return tlsRootCerts, nil
	}

	return nil, errors.Errorf("no application organization with MSP ID [%s] in channel [%s]", mspID, channelName)
}

// MSPManager returns the [driver.MSPManager] that reflects the current channel
// configuration. Users should not memoize references to this object.
//
// Obtaining a manager before a channel configuration is in force is allowed;
// the failure surfaces from the manager's own DeserializeIdentity, as
// [driver.ErrNotInitialized] or [driver.ErrConfigRejected].
//
// A manager obtained from a Service built with [Service.WithConfigWait] carries
// that budget: DeserializeIdentity waits up to that long for a configuration to
// arrive before reporting [driver.ErrNotInitialized].
func (c *Service) MSPManager() driver.MSPManager {
	return &mspManager{config: c.config, waitFor: c.configWait}
}

func (c *Service) CheckACL(signedProp driver.SignedProposal) error {
	return driver.ErrNotImplemented
}

type mspManager struct {
	config  *deferred.Holder[*configuration]
	waitFor time.Duration
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	res, err := m.resolve()
	if err != nil {
		return nil, err
	}

	return res.channelConfig.MSPManager().DeserializeIdentity(serializedIdentity)
}

// resolve returns the configuration in force, waiting for one to arrive if a
// wait budget was inherited from the [Service] that built this manager.
func (m *mspManager) resolve() (*configuration, error) {
	if m.waitFor <= 0 {
		return m.config.Get()
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.waitFor)
	defer cancel()
	return m.config.WaitForValue(ctx)
}
