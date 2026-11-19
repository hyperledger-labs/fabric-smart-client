/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	genericconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	fendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
	digendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig/support/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The node's own MSP, loaded by the MSP service at startup.
const ownMSPPath = "../msp/x509/testdata/msp"

// rotationCore is a core.yaml for node "me" with view identity at ownCert. It knows alice by
// her view identity at aliceCert, her alias alice_alias, and her Fabric identity at
// aliceFabricCert, which the Fabric resolver binds to her view identity by reusing her name.
const rotationCore = `
fsc:
  id: me
  grpc:
    address: 127.0.0.1:9000
  p2p:
    listenAddress: /ip4/127.0.0.1/tcp/9001
  endpoint:
    resolvers:
      - name: alice
        identity:
          path: ALICE_CERT
        addresses:
          P2P: 127.0.0.1:9101
        aliases:
          - alice_alias
fabric:
  default:
    default: true
    endpoint:
      resolvers:
        - name: alice
          identity:
            mspID: Org2MSP
            path: ALICE_FABRIC_CERT
`

type defaultIdentity view.Identity

func (d defaultIdentity) DefaultIdentity() view.Identity { return view.Identity(d) }

// rotatedNode is what a node knows about itself and alice after it started.
type rotatedNode struct {
	es          *endpoint.Service
	own         view.Identity
	ownMSP      view.Identity
	alice       view.Identity
	aliceFabric view.Identity
}

// startRotationNode starts node "me" on store the way the view and Fabric SDKs do: the view
// resolver loader registers the node's own resolver and alice's, then the Fabric resolver
// loader adds alice's Fabric identity, and the MSP service binds the node's MSP identity to
// its view identity. Startup must succeed.
func startRotationNode(t *testing.T, store driver.BindingStore, ownCert, aliceCert, aliceFabricCert string) *rotatedNode {
	t.Helper()
	dir := t.TempDir()
	core := strings.NewReplacer("ALICE_FABRIC_CERT", aliceFabricCert, "ALICE_CERT", aliceCert).Replace(rotationCore)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(core), 0o600))
	cp, err := config.NewProvider(dir)
	require.NoError(t, err)

	own, err := id.LoadIdentity(ownCert)
	require.NoError(t, err)
	es, err := endpoint.NewService(store)
	require.NoError(t, err)

	// view SDK Install
	loader, err := digendpoint.NewResolversLoader(cp, es, defaultIdentity(own))
	require.NoError(t, err)
	require.NoError(t, loader.LoadResolvers())

	// Fabric network start
	fabricConfig, err := genericconfig.NewService(cp, "default", true)
	require.NoError(t, err)
	resolvers, err := fendpoint.NewResolverService(fabricConfig, es)
	require.NoError(t, err)
	require.NoError(t, resolvers.LoadResolvers())

	kvsStore, err := mem.NewDriver().NewKVS("")
	require.NoError(t, err)
	kvss, err := kvs.New(kvsStore, "", kvs.DefaultCacheSize)
	require.NoError(t, err)
	mspService := msp.NewLocalMSPManager(fabricConfig, kvss, nil, es, own, sig.NewMultiplexDeserializer(), 100)
	require.NoError(t, mspService.RegisterX509MSP("default", ownMSPPath, "Org1MSP"))
	ownMSP, _, err := mspService.GetIdentityInfoByLabel(msp.BccspMSP, "default").GetIdentity(nil)
	require.NoError(t, err)

	alice, err := id.LoadIdentity(aliceCert)
	require.NoError(t, err)
	aliceFabric, err := x509.Serialize("Org2MSP", aliceFabricCert)
	require.NoError(t, err)
	return &rotatedNode{es: es, own: own, ownMSP: ownMSP, alice: alice, aliceFabric: aliceFabric}
}

// checkResolves checks that n resolves every identity it knows to its owner, and that the
// endorser's check that an endorsement signed by a Fabric identity comes from the party asked
// for it, IsBoundTo(Fabric identity, view identity), holds.
func (n *rotatedNode) checkResolves(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for _, c := range []struct {
		what     string
		id       view.Identity
		longTerm view.Identity
	}{
		{"alice's view identity", n.alice, n.alice},
		{"alice's alias", view.Identity("alice_alias"), n.alice},
		{"alice's Fabric identity", n.aliceFabric, n.alice},
		{"the node's own MSP identity", n.ownMSP, n.own},
	} {
		r, err := n.es.GetResolver(ctx, c.id)
		if assert.NoError(t, err, "GetResolver(%s)", c.what) {
			assert.Equal(t, c.longTerm, view.Identity(r.ID), "GetResolver(%s)", c.what)
		}
		if !c.id.Equal(c.longTerm) {
			assert.True(t, n.es.IsBoundTo(ctx, c.id, c.longTerm), "IsBoundTo(%s, its view identity)", c.what)
		}
	}
	aliasID, err := n.es.GetIdentity("alice_alias", nil)
	if assert.NoError(t, err, "GetIdentity(alice_alias)") {
		assert.Equal(t, n.alice, aliasID, "GetIdentity(alice_alias)")
	}
}

// An operator rotates a node's view identity by giving it a new certificate, putting the new
// certificate in the resolvers of every node that talks to it, and restarting them (#2559).
// The binding store is persistent. After the restart, every identity a node knows must resolve
// to its rotated owner, so that the node does not report a successful start and then reject
// the rotated node's endorsements.
func TestRestartAfterIdentityRotation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                     string
		rotateAlice, rotateOwner bool
		// oldRows plants the rows a node before this fix stored at startup.
		oldRows bool
	}{
		{name: "counterparty rotated", rotateAlice: true},
		{name: "own identity rotated", rotateOwner: true},
		{name: "both rotated, rows of an older version", rotateAlice: true, rotateOwner: true, oldRows: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newRotationStore(t)
			ownCert, aliceCert, aliceFabricCert := newCert(t), newCert(t), newCert(t)
			before := startRotationNode(t, store, ownCert, aliceCert, aliceFabricCert)
			before.checkResolves(t)
			if tc.oldRows {
				require.NoError(t, store.PutBindings(t.Context(), before.alice, before.aliceFabric, view.Identity("alice_alias")))
				require.NoError(t, store.PutBindings(t.Context(), before.own, before.ownMSP))
			}

			if tc.rotateAlice {
				aliceCert = newCert(t)
			}
			if tc.rotateOwner {
				ownCert = newCert(t)
			}
			after := startRotationNode(t, store, ownCert, aliceCert, aliceFabricCert)
			after.checkResolves(t)
		})
	}
}

// Replicas of a node share its binding store. While the configuration with alice's rotated
// identity rolls out, a replica that restarted with it and one still running, or restarted,
// with the old configuration each resolve alice's identities by their own configuration.
func TestReplicasDuringIdentityRotation(t *testing.T) {
	t.Parallel()
	store := newRotationStore(t)
	ownCert, aliceCert, aliceFabricCert := newCert(t), newCert(t), newCert(t)
	oldReplica := startRotationNode(t, store, ownCert, aliceCert, aliceFabricCert)

	newReplica := startRotationNode(t, store, ownCert, newCert(t), aliceFabricCert)
	newReplica.checkResolves(t)
	oldReplica.checkResolves(t)

	oldReplica = startRotationNode(t, store, ownCert, aliceCert, aliceFabricCert)
	oldReplica.checkResolves(t)
	newReplica.checkResolves(t)
}

// newRotationStore returns an empty binding store. The memory driver shares one SQLite
// database per process; a random table name keeps runs apart. Table names allow only letters.
func newRotationStore(t *testing.T) driver.BindingStore {
	t.Helper()
	table := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'a' + r - '0'
		}
		return r
	}, strings.ReplaceAll(utils.GenerateUUID(), "-", ""))
	store, err := mem.NewDriver().NewBinding("", "rotation", table)
	require.NoError(t, err)
	return store
}

func newCert(t *testing.T) string {
	t.Helper()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "cert.pem")
	require.NoError(t, os.WriteFile(path, kp.Cert, 0o600))
	return path
}
