/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/configstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Service answers membership questions about a channel from its current
// configuration. The configuration is not available when the service is built;
// it arrives with the first configuration block, via Update. Every accessor
// therefore reports driver.ErrNotInitialized until that happens.
type Service struct {
	// config holds the channel configuration once it has been loaded. Reading
	// it goes through configstate.Holder.Get, which cannot hand out a
	// configuration that is not there.
	config *configstate.Holder[*channelconfig.ChannelConfig]

	channelName string
}

func NewService(channelName string) *Service {
	return &Service{
		config:      configstate.NewHolder[*channelconfig.ChannelConfig](channelName),
		channelName: channelName,
	}
}

// Update installs the channel configuration carried by env. The previously held
// configuration is kept if env cannot be parsed.
func (c *Service) Update(env *cb.Envelope) error {
	return c.config.Update(func(*channelconfig.ChannelConfig, bool) (*channelconfig.ChannelConfig, error) {
		return parseConfig(env)
	})
}

func parseConfig(env *cb.Envelope) (*channelconfig.ChannelConfig, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get payload from config transaction")
	}

	cenv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	return channelconfig.NewChannelConfig(cenv.Config.ChannelGroup, factory.GetDefault())
}

func (c *Service) IsValid(identity view.Identity) error {
	res, err := c.config.Get()
	if err != nil {
		return err
	}

	id, err := res.MSPManager().DeserializeIdentity(identity)
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

	id, err := res.MSPManager().DeserializeIdentity(identity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed deserializing identity [%s]", identity.String())
	}

	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration. An empty result means the channel has no organizations; a
// channel whose configuration has not been loaded yet reports
// driver.ErrNotInitialized instead.
func (c *Service) GetMSPIDs() ([]string, error) {
	res, err := c.config.Get()
	if err != nil {
		return nil, err
	}

	var mspIDs []string
	if ac := res.ApplicationConfig(); ac != nil {
		for _, org := range ac.Organizations() {
			mspIDs = append(mspIDs, org.MSPID())
		}
	}

	return mspIDs, nil
}

// IsIdemixMSP reports whether the MSP identified by mspID is of type Idemix.
// A false result means the channel has such an MSP and it is not Idemix; a
// channel whose configuration has not been loaded yet reports
// driver.ErrNotInitialized instead, so a caller cannot mistake the startup
// race for a definitive answer.
func (c *Service) IsIdemixMSP(mspID string) (bool, error) {
	res, err := c.config.Get()
	if err != nil {
		return false, err
	}

	ac := res.ApplicationConfig()
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

	oc := res.OrdererConfig()
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
	config *configstate.Holder[*channelconfig.ChannelConfig]
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	res, err := m.config.Get()
	if err != nil {
		return nil, err
	}

	return res.MSPManager().DeserializeIdentity(serializedIdentity)
}
