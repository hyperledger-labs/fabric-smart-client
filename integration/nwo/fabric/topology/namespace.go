/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package topology

import "github.com/onsi/gomega"

const (
	// BaseChaincodeImage is the CCaaS image of the built-in chaincode a
	// namespace uses when it names neither an image nor a source path.
	BaseChaincodeImage = "fsc-cc/base:latest"

	// StateQueryChaincodePath is the Go import path of FSC's rich-query
	// chaincode, for use with WithLegacyChaincode.
	StateQueryChaincodePath = "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state/cc/query"

	defaultVersion  = "Version-0.0"
	defaultSequence = "1"
	defaultCtor     = `{"Args":["init"]}`
)

// Namespace is the handle AddNamespace returns. Options mutate it during
// construction; afterwards it exposes the few post-construction operations
// tests need.
// Options must be order-independent, so each records that it ran and the
// defaults are settled after they have all been applied.
type Namespace struct {
	cc *ChannelChaincode

	peersSet     bool // WithPeers ran, so peer derivation is skipped
	ctorSet      bool // WithCtor ran, so no default ctor
	chaincodeSet bool // WithContainerImage or WithLegacyChaincode ran
}

// NamespaceOption customises a namespace at construction time.
type NamespaceOption func(*Namespace)

// WithContainerImage deploys the namespace's chaincode as a container built
// from ref. nwo starts one server per organization; the image must already
// exist locally (`make chaincode-images`).
func WithContainerImage(ref string) NamespaceOption {
	return func(n *Namespace) {
		n.cc.Chaincode.Image = ref
		n.cc.Chaincode.Path = ""
		n.cc.Chaincode.Lang = ""
		n.chaincodeSet = true
	}
}

// WithLegacyChaincode packages the Go source at goPkg and lets the peer build
// it in a ccenv-derived container.
func WithLegacyChaincode(goPkg string) NamespaceOption {
	return func(n *Namespace) {
		n.cc.Chaincode.Path = goPkg
		n.cc.Chaincode.Lang = "golang"
		n.cc.Chaincode.Image = ""
		n.chaincodeSet = true
	}
}

// WithPackageFile installs a lifecycle package the caller has already built
// instead of packaging the source. Legacy deployments only.
func WithPackageFile(path string) NamespaceOption {
	return func(n *Namespace) { n.cc.Chaincode.PackageFile = path }
}

// WithCtor sets the chaincode's init arguments and marks init as required.
func WithCtor(ctor string) NamespaceOption {
	return func(n *Namespace) {
		n.cc.Chaincode.Ctor = ctor
		n.ctorSet = true
	}
}

// WithPeers pins the peers that host the namespace, overriding the peers
// derived from the policy's organizations.
func WithPeers(peers ...string) NamespaceOption {
	return func(n *Namespace) {
		n.cc.Peers = peers
		n.peersSet = true
	}
}

// WithVersion overrides the chaincode version.
func WithVersion(v string) NamespaceOption {
	return func(n *Namespace) { n.cc.Chaincode.Version = v }
}

// AddPostRunInvocation queues an invocation to run once the network is up.
func (n *Namespace) AddPostRunInvocation(
	functionName string, expectedResult any, args ...[]byte,
) *Namespace {
	n.cc.AddPostRunInvocation(functionName, expectedResult, args...)
	return n
}

// AddNamespace declares a chaincode namespace on the topology's first channel.
// By default it deploys the built-in base chaincode as a container; supply
// WithContainerImage or WithLegacyChaincode to deploy something else.
//
// Peers are resolved here, not at deploy time: FSC nodes are appended to the
// network's peer list later, under the same organization names, and must not
// receive chaincode.
func (t *Topology) AddNamespace(
	name string, policy EndorsementPolicy, opts ...NamespaceOption,
) *Namespace {
	gomega.Expect(t.Channels).NotTo(gomega.BeEmpty(),
		"topology has no channel to add namespace [%s] to", name)

	ns := &Namespace{cc: &ChannelChaincode{
		Chaincode: Chaincode{
			Name:            name,
			Label:           name,
			Version:         defaultVersion,
			Sequence:        defaultSequence,
			Image:           BaseChaincodeImage,
			SignaturePolicy: policy.Rule(),
		},
		Channel: t.Channels[0].Name,
	}}

	for _, opt := range opts {
		opt(ns)
	}

	if !ns.peersSet {
		ns.cc.Peers = t.peersForPolicy(policy, ns.cc.Channel)
	}
	// The built-in base chaincode expects an init; a chaincode the caller
	// named does not, unless it asked for one. Settled after the options so
	// the result does not depend on the order they were written in.
	if !ns.ctorSet && !ns.chaincodeSet {
		ns.cc.Chaincode.Ctor = defaultCtor
	}
	ns.cc.Chaincode.InitRequired = ns.cc.Chaincode.Ctor != ""

	t.validateNamespace(ns, policy)
	t.AddChaincode(ns.cc)

	return ns
}

// peersForPolicy returns the peers of the policy's organizations, or every
// peer on the channel when the policy names none.
func (t *Topology) peersForPolicy(policy EndorsementPolicy, channel string) []string {
	orgs := policy.Orgs()
	if len(orgs) == 0 {
		var peers []string
		for _, p := range t.Peers {
			if p.onChannel(channel) {
				peers = append(peers, p.Name)
			}
		}
		return peers
	}

	var peers []string
	for _, org := range orgs {
		for _, p := range t.Peers {
			if p.Organization == org {
				peers = append(peers, p.Name)
			}
		}
	}
	return peers
}

// ApplyOptions applies namespace options to an existing chaincode. It is how
// UpdateChaincode reuses the same vocabulary as AddNamespace.
func ApplyOptions(cc *ChannelChaincode, opts ...NamespaceOption) {
	ns := &Namespace{cc: cc}
	for _, opt := range opts {
		opt(ns)
	}
	cc.Chaincode.InitRequired = cc.Chaincode.Ctor != ""
}

func (t *Topology) validateNamespace(ns *Namespace, policy EndorsementPolicy) {
	cc := &ns.cc.Chaincode
	name := cc.Name

	gomega.Expect(cc.Image != "" && cc.Path != "").To(gomega.BeFalse(),
		"namespace [%s] cannot be both a container image and a source path", name)
	gomega.Expect(cc.PackageFile != "" && cc.IsCCaaS()).To(gomega.BeFalse(),
		"namespace [%s] cannot combine WithPackageFile with WithContainerImage", name)
	gomega.Expect(ns.cc.Peers).NotTo(gomega.BeEmpty(),
		"namespace [%s] resolved to no peers", name)

	for _, org := range policy.Orgs() {
		gomega.Expect(t.hasOrganization(org)).To(gomega.BeTrue(),
			"namespace [%s] policy names organization [%s], which is not in the topology", name, org)
	}
}

// hasOrganization reports whether the topology declares an organization with
// this name.
func (t *Topology) hasOrganization(name string) bool {
	for _, o := range t.Organizations {
		if o.Name == name {
			return true
		}
	}
	return false
}
