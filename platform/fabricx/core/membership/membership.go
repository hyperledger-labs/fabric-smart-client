/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"slices"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	m "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/common/configtx"
	"github.com/hyperledger/fabric-x-common/common/policies"
	"github.com/hyperledger/fabric-x-common/core/aclmgmt"
	"github.com/hyperledger/fabric-x-common/core/aclmgmt/resources"
	"github.com/hyperledger/fabric-x-common/core/policy"
	xmsp "github.com/hyperledger/fabric-x-common/msp"
	"github.com/hyperledger/fabric-x-common/msp/mgmt"
	"github.com/hyperledger/fabric-x-common/protoutil"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = logging.MustGetLogger()

// Service answers membership questions about a channel from its current
// configuration. The configuration is not available when the service is built;
// it arrives with the first poll of the channel config monitor, via Update.
// Until one is in force every accessor reports driver.ErrNotInitialized, or,
// once a configuration has arrived and failed validation,
// driver.ErrConfigRejected.
type Service struct {
	// config holds the channel configuration once it has been loaded. Reading
	// it goes through deferred.Holder.Get, which cannot hand out a
	// configuration that is not there.
	config *deferred.Holder[channelconfig.Resources]

	ACLProvider aclmgmt.ACLProvider

	channelID string
}

func NewService(channelID string) *Service {
	s := &Service{
		config:    deferred.NewHolder[channelconfig.Resources]("channel [" + channelID + "] configuration"),
		channelID: channelID,
	}
	policyChecker := policy.NewPolicyChecker(
		&policyManagerGetterFunc{channelID: channelID, config: s.config},
		mgmt.GetLocalMSP(factory.GetDefault()),
	)
	s.ACLProvider = aclmgmt.NewACLProvider(
		func(cid string) channelconfig.Resources {
			if cid != s.channelID {
				return nil
			}
			// A channel whose configuration has not loaded yet is reported the
			// same way as an unknown channel. CheckACL rejects that case before
			// delegating here, so this is a backstop rather than the guard.
			res, ok := s.config.TryGet()
			if !ok {
				return nil
			}
			return res
		},
		policyChecker,
	)

	return s
}

// Update installs the channel configuration carried by env. The previously held
// configuration is kept if env fails validation.
func (s *Service) Update(env *cb.Envelope) error {
	logger.Infof("updating channel [%s]", s.channelID)

	err := s.config.Update(func(current channelconfig.Resources, loaded bool) (channelconfig.Resources, error) {
		return s.validateConfig(env, current, loaded)
	})
	if err != nil {
		logger.Errorf("failed validating config for channel [%s]: [%s]", s.channelID, err)
		return err
	}

	logger.Infof("updating channel [%s], done", s.channelID)
	return nil
}

// DryUpdate validates env against the currently held configuration without
// installing it.
//
// The configuration is snapshotted rather than locked for the duration of the
// validation: a held bundle is never mutated, only replaced wholesale, so the
// snapshot stays consistent. The verdict is advisory either way, since an
// Update may land the moment this returns.
func (s *Service) DryUpdate(env *cb.Envelope) error {
	current, loaded := s.config.TryGet()

	_, err := s.validateConfig(env, current, loaded)
	return err
}

// validateConfig parses env and checks it against current, the configuration in
// force. It is called while the holder's lock is held, so it takes the current
// configuration as an argument rather than reading it back out.
func (s *Service) validateConfig(env *cb.Envelope, current channelconfig.Resources, loaded bool) (*channelconfig.Bundle, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal common payload")
	}

	cenv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal config envelope")
	}

	// The first configuration has nothing to be validated against.
	if loaded {
		if err := current.ConfigtxValidator().Validate(cenv); err != nil {
			return nil, errors.Wrap(err, "validate config transaction")
		}
	}

	bundle, err := channelconfig.NewBundle(s.channelID, cenv.Config, factory.GetDefault())
	if err != nil {
		return nil, errors.Wrapf(err, "build a new bundle")
	}

	channelconfig.LogSanityChecks(bundle)
	if err := capabilitiesSupported(bundle); err != nil {
		return nil, errors.Wrapf(err, "check bundle capabilities")
	}

	return bundle, nil
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] application config capabilities incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] channel config capabilities incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}

func toMSPIdentity(identity view.Identity) (*msppb.Identity, error) {
	sId := &m.SerializedIdentity{}
	err := proto.Unmarshal(identity, sId)
	if err != nil {
		return nil, err
	}

	sid := &msppb.Identity{
		MspId: sId.GetMspid(),
		Creator: &msppb.Identity_Certificate{
			Certificate: sId.GetIdBytes(),
		},
	}

	return sid, nil
}

func (s *Service) IsValid(identity view.Identity) error {
	res, err := s.config.Get()
	if err != nil {
		return err
	}

	sid, err := toMSPIdentity(identity)
	if err != nil {
		return err
	}

	id, err := res.MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return errors.Wrapf(err, "deserializing identity [%s]", identity.String())
	}

	return id.Validate()
}

func (s *Service) GetVerifier(identity view.Identity) (driver.Verifier, error) {
	res, err := s.config.Get()
	if err != nil {
		return nil, err
	}

	sid, err := toMSPIdentity(identity)
	if err != nil {
		return nil, err
	}

	id, err := res.MSPManager().DeserializeIdentity(sid)
	if err != nil {
		return nil, errors.Wrapf(err, "deserializing identity [%s]", identity.String())
	}

	return id, nil
}

// GetMSPIDs retrieves the MSP IDs of the organizations in the current Channel
// configuration. An empty result means the channel has no organizations; a
// channel with no configuration in force reports driver.ErrNotInitialized or
// driver.ErrConfigRejected instead.
func (s *Service) GetMSPIDs() ([]string, error) {
	res, err := s.config.Get()
	if err != nil {
		return nil, err
	}

	var mspIDs []string
	if ac, ok := res.ApplicationConfig(); ok {
		for _, org := range ac.Organizations() {
			mspIDs = append(mspIDs, org.MSPID())
		}
	}

	return mspIDs, nil
}

func (s *Service) OrdererConfig(cs driver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	res, err := s.config.Get()
	if err != nil {
		return "", nil, err
	}

	oc, ok := res.OrdererConfig()
	if !ok || oc.Organizations() == nil {
		return "", nil, errors.Errorf("orderer config does not exist for channel [%s]", s.channelID)
	}

	// Discovered trust anchors augment the network's configured pool rather than replacing
	// it; see the equivalent in platform/fabric.
	networkTLS := cs.NetworkClientTLS()
	connectionTimeout := cs.ClientConnTimeout()

	var newOrderers []*grpc.ConnectionConfig
	orgs := oc.Organizations()
	for _, org := range orgs {
		m := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, m.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, m.GetTLSIntermediateCerts()...)
		for _, epStr := range org.Endpoints() {
			if len(epStr) == 0 {
				continue
			}

			ep, err := parseEndpoint(epStr)
			if err != nil {
				return "", nil, errors.Wrapf(err, "parse orderer endpoint [%s]", epStr)
			}

			// Skip deliver-typed endpoints; FSC clients only broadcast here.
			// Deliver is sourced from the local sidecar / committer.
			if ep.Type != OrdererBroadcastType {
				continue
			}

			endpointTLS := networkTLS
			endpointTLS.ServerRootCAs = append(
				slices.Clone(networkTLS.ServerRootCAs), tlsRootCerts...)
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           ep.Endpoint,
				ConnectionTimeout: connectionTimeout,
				TLS:               endpointTLS,
				Usage:             ep.Type,
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
func (s *Service) MSPManager() driver.MSPManager {
	return &mspManager{config: s.config}
}

// IsIdemixMSP reports whether the MSP identified by mspID is of type Idemix.
// A false result means the channel has such an MSP and it is not Idemix; a
// channel with no configuration in force reports driver.ErrNotInitialized or
// driver.ErrConfigRejected instead, so a caller cannot mistake an absent
// configuration for a definitive answer.
func (s *Service) IsIdemixMSP(mspID string) (bool, error) {
	res, err := s.config.Get()
	if err != nil {
		return false, err
	}

	ac, ok := res.ApplicationConfig()
	if !ok {
		return false, nil
	}

	for _, org := range ac.Organizations() {
		if org.MSPID() == mspID {
			return org.MSP().GetType() == xmsp.IDEMIX, nil
		}
	}

	return false, nil
}

// ConfigSequence returns the sequence number of the channel configuration
// currently in force. It reflects the sequence of the configuration bundle
// built by the config monitor and increases by one for every configuration
// update this node has applied. A channel with no configuration in force
// reports driver.ErrNotInitialized or driver.ErrConfigRejected instead.
func (s *Service) ConfigSequence() (uint64, error) {
	res, err := s.config.Get()
	if err != nil {
		return 0, err
	}

	return res.ConfigtxValidator().Sequence(), nil
}

// CheckACL checks the ACL for the resource for the Channel using the
// SignedProposal from which an id can be extracted for testing against a policy
func (s *Service) CheckACL(signedProp driver.SignedProposal) error {
	// Reject before delegating: the ACL provider expresses "no configuration"
	// as a policy lookup failure, which would not tell the caller that the
	// channel simply has not started up yet.
	if _, err := s.config.Get(); err != nil {
		return err
	}

	return s.ACLProvider.CheckACL(resources.Peer_Propose, s.channelID, signedProp.Internal())
}

type mspManager struct {
	config *deferred.Holder[channelconfig.Resources]
}

func (m *mspManager) DeserializeIdentity(serializedIdentity []byte) (driver.MSPIdentity, error) {
	res, err := m.config.Get()
	if err != nil {
		return nil, err
	}

	sid, err := toMSPIdentity(serializedIdentity)
	if err != nil {
		return nil, err
	}

	return res.MSPManager().DeserializeIdentity(sid)
}

type policyManagerGetterFunc struct {
	channelID string
	config    *deferred.Holder[channelconfig.Resources]
}

func (p *policyManagerGetterFunc) Manager(channelID string) policies.Manager {
	if p.channelID != channelID {
		return nil
	}

	res, ok := p.config.TryGet()
	if !ok {
		return nil
	}

	return res.PolicyManager()
}
