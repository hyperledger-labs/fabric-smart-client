/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type IdentityOptions struct {
	EIDExtension bool
	AuditInfo    []byte
}

type GetIdentityFunc func(opts *IdentityOptions) (view.Identity, []byte, error)

type IdentityInfo struct {
	ID           string
	EnrollmentID string
	GetIdentity  GetIdentityFunc
}

type LocalMembership interface {
	DefaultIdentity() view.Identity
	AnonymousIdentity() (view.Identity, error)
	IsMe(ctx context.Context, id view.Identity) bool
	DefaultSigningIdentity() SigningIdentity
	RegisterX509MSP(id, path, mspID string) error
	RegisterIdemixMSP(id, path, mspID string) error
	GetIdentityByID(id string) (view.Identity, error)
	GetIdentityInfoByLabel(mspType, label string) *IdentityInfo
	GetIdentityInfoByIdentity(mspType string, id view.Identity) *IdentityInfo
	Refresh() error
}

type MSPIdentity interface {
	GetMSPIdentifier() string
	Validate() error
	Verify(message, sigma []byte) error
}

// MSPManager resolves serialized identities against the MSPs of a channel.
//
// A manager reflects the channel configuration in force at the time each of its
// methods is called, not the configuration in force when it was obtained, so
// callers should not memoize one.
type MSPManager interface {
	// DeserializeIdentity turns a serialized identity into an MSPIdentity
	// belonging to one of the channel's MSPs. It fails if the identity does not
	// belong to any of them, and reports ErrNotInitialized or ErrConfigRejected
	// while the channel has no configuration in force.
	DeserializeIdentity(serializedIdentity []byte) (MSPIdentity, error)
}

// ChannelMembership answers membership questions about a channel from its
// current configuration: which organizations belong to it, and whether a given
// identity is one the channel recognizes.
//
// A channel's configuration is not available as soon as the channel exists: it
// is loaded asynchronously, with the first configuration block on Fabric and
// with the first successful poll of the config monitor on Fabric-x. Until a
// configuration is in force, every method here reports an error rather than
// answering from an empty one, and names which of two situations applies:
//
//   - ErrNotInitialized, while no configuration has arrived. A caller racing
//     node startup can detect this with errors.Is and retry; treating it as a
//     permanent failure will misreport a node that is merely still coming up.
//   - ErrConfigRejected, once one has arrived and been refused. Retrying will
//     not clear this, so a caller should surface it — but the channel is not
//     finished either, and recovers if a later configuration is accepted.
//
// Both are absent-configuration answers; they differ only in whether retrying
// can help, so a caller that cannot act on the difference may test for either.
type ChannelMembership interface {
	// GetMSPIDs returns the MSP IDs of the organizations in the current channel
	// configuration. An empty result means the channel has no organizations; a
	// channel with no configuration in force reports one of the errors above
	// instead.
	GetMSPIDs() ([]string, error)
	// MSPManager returns a manager backed by the channel's current
	// configuration. It never returns nil and may be called before a
	// configuration is in force; that failure surfaces from the manager's own
	// methods. Callers should not memoize the returned value.
	MSPManager() MSPManager
	// IsValid reports whether identity is one the channel recognizes, by
	// deserializing it against the channel's MSPs and validating it. A nil
	// error means the identity is valid. It reports one of the errors above
	// while the channel has no configuration in force.
	IsValid(identity view.Identity) error
	// GetVerifier returns a Verifier that checks signatures produced by
	// identity. It fails if identity is not one the channel recognizes, and
	// reports one of the errors above while the channel has no configuration in
	// force.
	GetVerifier(identity view.Identity) (Verifier, error)
	// CheckACL checks the ACL for the resource for the Channel using the
	// SignedProposal from which an id can be extracted for testing against a
	// policy. Implementations that do not enforce ACLs return
	// ErrNotImplemented; those that do report one of the errors above while the
	// channel has no configuration in force.
	CheckACL(signedProp SignedProposal) error
	// IsIdemixMSP reports whether the MSP with the given ID is of type Idemix.
	// A false result means the channel has such an MSP and it is not Idemix; a
	// channel with no configuration in force reports one of the errors above
	// instead, so callers choosing an identity encoding from this cannot
	// mistake an absent configuration for a real answer.
	IsIdemixMSP(mspID string) (bool, error)
	// ConfigSequence returns the sequence number of the channel configuration
	// currently in force. It is 0 for a channel's genesis configuration and
	// increases by one for every configuration update the node has applied, so
	// a caller can tell whether a configuration change has reached this node
	// yet. A channel with no configuration in force reports one of the errors
	// above instead.
	ConfigSequence() (uint64, error)
}

// MembershipService is the driver-side view of a channel's membership: a
// ChannelMembership that also owns the configuration the answers come from.
//
// Reading membership is the concern of ChannelMembership; the two methods added
// here belong to whoever feeds the channel configuration in and configures the
// ordering service from it, which is the committer on Fabric and the channel
// config monitor on Fabric-x.
type MembershipService interface {
	ChannelMembership
	// Update installs the channel configuration carried by env, replacing the
	// one currently in force. If env is rejected, the previously installed
	// configuration is left in place, so a bad update cannot leave the service
	// with no configuration at all.
	Update(env *common.Envelope) error
	// OrdererConfig returns the consensus type and the orderer endpoints in the
	// current channel configuration, with the TLS settings from cs applied. It
	// fails if the configuration carries no orderer section, and reports
	// ErrNotInitialized or ErrConfigRejected while none is in force.
	OrdererConfig(cs ConfigService) (string, []*grpc.ConnectionConfig, error)
}
