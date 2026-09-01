/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
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
// it arrives with the first configuration block, via Update. Until one is in
// force every accessor reports driver.ErrNotInitialized, or, once a block has
// arrived and been refused, driver.ErrConfigRejected.
type Service struct {
	// config holds the channel configuration once it has been loaded. Reading
	// it goes through deferred.Holder.Get, which cannot hand out a
	// configuration that is not there.
	config *deferred.Holder[*configuration]

	channelName string
}

func NewService(channelName string) *Service {
	return &Service{
		config:      deferred.NewHolder[*configuration]("channel [" + channelName + "] configuration"),
		channelName: channelName,
	}
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
	res, err := c.config.Get()
	if err != nil {
		return 0, err
	}

	return res.sequence, nil
}

func (c *Service) IsValid(identity view.Identity) error {
	res, err := c.config.Get()
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
	res, err := c.config.Get()
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
	res, err := c.config.Get()
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
	res, err := c.config.Get()
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
	res, err := c.config.Get()
	if err != nil {
		return "", nil, err
	}

	oc := res.channelConfig.OrdererConfig()
	if oc == nil {
		return "", nil, errors.Errorf("orderer config does not exist for channel [%s]", c.channelName)
	}

	tlsEnabled, isSet := cs.OrderingTLSEnabled()
	if !isSet {
		tlsEnabled = cs.TLSEnabled()
	}
	tlsClientSideAuth, isSet := cs.OrderingTLSClientAuthRequired()
	if !isSet {
		tlsClientSideAuth = cs.TLSClientAuthRequired()
	}
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
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           endpoint,
				ConnectionTimeout: connectionTimeout,
				TLSEnabled:        tlsEnabled,
				TLSClientSideAuth: tlsClientSideAuth,
				TLSRootCertBytes:  tlsRootCerts,
			})
		}
	}

	return oc.ConsensusType(), newOrderers, nil
}

// TLSRootCertsByMSPID returns the TLS root and intermediate certificates of the
// application organization with the given MSP ID, taken from the channel
// configuration in force.
//
// This is the trusted counterpart to the TLS certificates carried in a
// discovery response's ConfigResult. That response is not independently
// verified — the peer answering a discovery query supplies both the endpoints
// and the certificates that would authenticate them — so a client that dials a
// discovered peer with roots from the same response gets no authentication from
// TLS at all. Sourcing them here instead means the certificates come from the
// channel configuration, fetched from a locally configured peer, rather than
// from whoever answered discovery.
//
// It reports driver.ErrNotInitialized or driver.ErrConfigRejected while no
// configuration is in force, and fails if the configuration has no application
// organization with this MSP ID, so an unknown organization cannot be mistaken
// for one that has no certificates configured.
func (c *Service) TLSRootCertsByMSPID(mspID string) ([][]byte, error) {
	res, err := c.config.Get()
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
// and returns its TLS root certificates followed by its intermediate ones.
//
// It takes the channelconfig.Application interface rather than reading the
// configuration itself so that the lookup can be tested directly: the concrete
// *channelconfig.ApplicationConfig cannot be built with organizations from
// outside its own package.
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

// MSPManager returns the driver.MSPManager that reflects the current Channel
// configuration. Users should not memoize references to this object.
//
// The manager resolves the configuration on each call rather than capturing it
// here, so obtaining one before the channel configuration has been loaded is
// allowed; the failure surfaces from DeserializeIdentity.
func (c *Service) MSPManager() driver.MSPManager {
	return &mspManager{config: c.config}
}

func (c *Service) CheckACL(signedProp driver.SignedProposal) error {
	return driver.ErrNotImplemented
}

type mspManager struct {
	config *deferred.Holder[*configuration]
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	res, err := m.config.Get()
	if err != nil {
		return nil, err
	}

	return res.channelConfig.MSPManager().DeserializeIdentity(serializedIdentity)
}
