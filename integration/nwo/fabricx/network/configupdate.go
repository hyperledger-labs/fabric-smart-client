/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	protosorderer "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/hyperledger/fabric-x-common/cmd/common/signer"
	"github.com/hyperledger/fabric-x-common/common/channelconfig"
	"github.com/hyperledger/fabric-x-common/protoutil"
	"github.com/hyperledger/fabric-x-common/tools/configtxlator/update"
	"github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	fabric_network "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"
)

// configRequestTimeout bounds every gRPC call the configuration helpers make.
const configRequestTimeout = 30 * time.Second

// clientCredentials returns the transport credentials needed to reach the committer's
// query service and the ordering service. Both require mTLS, and accept any identity
// signed by a fabric organization CA, so the credentials of the first FSC peer are
// borrowed -- the same thing extensions/scv2/ext.go does when it generates node
// configuration.
func clientCredentials(n *Network) credentials.TransportCredentials {
	var fscPeer *topology.Peer
	for _, p := range n.Peers {
		if p.Type == topology.FSCPeer {
			fscPeer = p
			break
		}
	}
	gomega.Expect(fscPeer).NotTo(gomega.BeNil(), "the topology has no FSC peer to borrow TLS credentials from")

	tlsDir := n.PeerLocalTLSDir(fscPeer)
	cert, err := tls.LoadX509KeyPair(filepath.Join(tlsDir, "server.crt"), filepath.Join(tlsDir, "server.key"))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot load the client TLS key pair")

	bundle, err := os.ReadFile(n.CACertsBundlePath())
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot read the CA certificate bundle")

	pool := x509.NewCertPool()
	gomega.Expect(pool.AppendCertsFromPEM(bundle)).To(gomega.BeTrue(), "the CA certificate bundle holds no certificate")

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	})
}

// GetConfig returns the channel configuration the committer currently holds, whose
// [common.Config.GetSequence] advances by one for every update the channel has applied.
//
// Every call queries the committer over the network, bounded by configRequestTimeout, and
// no result is cached, so a caller waiting for a configuration change can poll this. An
// unreachable query service, or a reply carrying no configuration, aborts the running spec
// through Gomega.
//
// It takes no channel argument because a committer test node serves exactly one channel,
// fixed when its container starts.
func GetConfig(n *Network) *cb.Config {
	committer := n.Peer(n.CommitterOrg, n.CommitterName)
	gomega.Expect(committer).NotTo(gomega.BeNil(),
		"no committer peer [%s.%s] in the topology", n.CommitterOrg, n.CommitterName)

	addr := fmt.Sprintf("127.0.0.1:%d", n.PeerPort(committer, QueryServicePortName))
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(clientCredentials(n)))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot reach the query service at [%s]", addr)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), configRequestTimeout)
	defer cancel()

	res, err := committerpb.NewQueryServiceClient(conn).GetConfigTransaction(ctx, &emptypb.Empty{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the query service reported no config transaction")

	env := &cb.Envelope{}
	gomega.Expect(proto.Unmarshal(res.GetEnvelope(), env)).NotTo(gomega.HaveOccurred())

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	cenv, err := protoutil.UnmarshalConfigEnvelope(payload.Data)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(cenv.GetConfig()).NotTo(gomega.BeNil(), "the config envelope carries no configuration")

	return cenv.GetConfig()
}

// UpdateConfig computes the difference between two channel configurations, signs it as the
// orderer organization's admin plus, for each organization named in orgs, that
// organization's peer Admin, and broadcasts the resulting CONFIG envelope to the ordering
// service.
//
// The orderer admin alone satisfies an Orderer-group value's Admins policy (ImplicitMeta
// MAJORITY Admins over the Orderer group's own, single organization) -- the only kind of
// change callers needed until this parameter existed. An Application- or Channel-group
// value is instead gated by a MAJORITY Admins policy over the Application group's peer
// organizations, which no orderer identity can satisfy; name enough of those organizations
// in orgs to reach that majority. The orderer admin remains the envelope's creator and
// outer signer either way -- orgs only adds the additional ConfigSignatures a
// peer-org-gated value's ModPolicy requires.
//
// It returns once the ordering service has accepted the envelope, which is earlier than
// the committer having applied it: a caller that reads the configuration back immediately
// may still observe the one it replaced. To wait for the new configuration to be in force,
// poll [GetConfig] for a higher sequence.
//
// The envelope is derived through the current configuration's own configtx validator, the
// same check FSC's membership service re-runs on receipt, so an envelope accepted here is
// one the nodes under test should accept as well. An update the current configuration
// rejects, or a broadcast the ordering service refuses, aborts the running spec through
// Gomega.
//
// Fabric-X has no component that does this on a client's behalf, which is why it lives
// here: the mock orderer in the committer test node only batches what is broadcast to it.
func UpdateConfig(n *Network, o *topology.Orderer, channel string, current, updated *cb.Config, orgs ...string) {
	// The Orderer group's Admins policy gates an Orderer-group change, so the orderer's
	// admin signs it.
	admin := adminSigner(n, o)

	configUpdate, err := update.Compute(current, updated)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot compute the configuration update")
	configUpdate.ChannelId = channel

	updateEnvelope := &cb.ConfigUpdateEnvelope{ConfigUpdate: protoutil.MarshalOrPanic(configUpdate)}
	updateEnvelope.Signatures = []*cb.ConfigSignature{signConfigUpdate(admin, updateEnvelope.ConfigUpdate)}
	for _, org := range orgs {
		updateEnvelope.Signatures = append(updateEnvelope.Signatures,
			signConfigUpdate(peerAdminSigner(n, org), updateEnvelope.ConfigUpdate))
	}

	signedUpdate, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG_UPDATE, channel, admin, updateEnvelope, 0, 0)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot sign the configuration update")

	bundle, err := channelconfig.NewBundle(channel, current, factory.GetDefault())
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot build a bundle from the current configuration")

	// ProposeConfigUpdate authorizes the update against the current configuration's mod
	// policies and returns the configuration that results from applying it, with its
	// sequence advanced by one.
	configEnvelope, err := bundle.ConfigtxValidator().ProposeConfigUpdate(signedUpdate)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the current configuration rejected the update")

	broadcast(n, o, signedConfigTx(admin, channel, configEnvelope))
}

// signedConfigTx wraps a ConfigEnvelope into the CONFIG envelope broadcast to the ordering
// service.
//
// The envelope is assembled by hand rather than through protoutil.CreateSignedEnvelope
// because that helper leaves the channel header's TxId empty, and the committer's sidecar
// checks for a transaction ID (service/sidecar/mapping.go:119) before it dispatches on
// header type (:125). A CONFIG envelope without one is excluded as
// MALFORMED_MISSING_TX_ID, the config block commits carrying zero transactions, nothing is
// logged at INFO level, and the configuration simply never changes.
func signedConfigTx(admin *signer.Signer, channel string, configEnvelope *cb.ConfigEnvelope) *cb.Envelope {
	creator, err := admin.Serialize()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot serialize the signing identity")

	nonce, err := protoutil.CreateNonce()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot create a nonce")

	signatureHeader := &cb.SignatureHeader{Creator: creator, Nonce: nonce}
	channelHeader := protoutil.MakeChannelHeader(cb.HeaderType_CONFIG, 0, channel, 0)
	protoutil.SetTxID(channelHeader, signatureHeader)

	payload := protoutil.MarshalOrPanic(&cb.Payload{
		Header: protoutil.MakePayloadHeader(channelHeader, signatureHeader),
		Data:   protoutil.MarshalOrPanic(configEnvelope),
	})

	signature, err := admin.Sign(payload)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot sign the configuration transaction")

	return &cb.Envelope{Payload: payload, Signature: signature}
}

// adminSigner is the given orderer organization's Admin identity.
func adminSigner(n *Network, o *topology.Orderer) *signer.Signer {
	org := n.Organization(o.Organization)
	gomega.Expect(org).NotTo(gomega.BeNil(), "orderer [%s] belongs to no known organization", o.Name)

	s, err := signer.NewSigner(signer.Config{
		MSPID:        org.MSPID,
		IdentityPath: n.OrdererUserCert(o, "Admin"),
		KeyPath:      n.OrdererUserKey(o, "Admin"),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot load the orderer admin signing identity")

	return s
}

// peerAdminSigner is the given organization's peer Admin identity.
//
// It mirrors adminSigner, but resolves credentials through the organization's first peer
// rather than an orderer: Fabric-X strips Fabric peers from the running network, yet keeps
// their crypto material and topology.Peer registration, which is the same pattern
// createNSCommon already relies on (via PeersInOrg and PeerUserMSPDir) to reach a peer
// organization's Admin identity for namespace deployment. It aborts the running spec
// through Gomega if the organization has no peers or no known identity.
func peerAdminSigner(n *Network, org string) *signer.Signer {
	peers := n.PeersInOrg(org)
	gomega.Expect(peers).NotTo(gomega.BeEmpty(), "organization [%s] has no peers to sign as its Admin", org)

	organization := n.Organization(org)
	gomega.Expect(organization).NotTo(gomega.BeNil(), "no known organization [%s]", org)

	s, err := signer.NewSigner(signer.Config{
		MSPID:        organization.MSPID,
		IdentityPath: n.PeerUserCert(peers[0], "Admin"),
		KeyPath:      n.PeerUserKey(peers[0], "Admin"),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot load the [%s] admin signing identity", org)

	return s
}

// signConfigUpdate produces one admin signature over a marshalled ConfigUpdate.
//
// The signing input is the concatenation of the signature header and the ConfigUpdate, in
// that order, which is what Fabric's configtx validator reconstructs to verify the
// signature. slices.Concat rather than append: appending to the marshalled header would
// write into its backing array if it ever had spare capacity, corrupting the header this
// same signature covers.
func signConfigUpdate(admin *signer.Signer, configUpdate []byte) *cb.ConfigSignature {
	header, err := protoutil.NewSignatureHeader(admin)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot build a signature header")

	signature := &cb.ConfigSignature{SignatureHeader: protoutil.MarshalOrPanic(header)}
	signature.Signature, err = admin.Sign(slices.Concat(signature.SignatureHeader, configUpdate))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot sign the configuration update")

	return signature
}

// broadcast submits an envelope to the ordering service and waits for its status.
//
// It dials the orderer itself rather than through fabric_network.Broadcast: that helper
// dials with RequireClientCert false, and the fabricx topology sets ClientAuthRequired,
// so the orderer closes the connection with "tls: certificate required".
func broadcast(n *Network, o *topology.Orderer, env *cb.Envelope) {
	addr := n.OrdererAddress(o, fabric_network.ListenPort)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(clientCredentials(n)))
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot reach the ordering service at [%s]", addr)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), configRequestTimeout)
	defer cancel()

	stream, err := protosorderer.NewAtomicBroadcastClient(conn).Broadcast(ctx)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "cannot open a broadcast stream")

	gomega.Expect(stream.Send(env)).NotTo(gomega.HaveOccurred(), "cannot send the configuration transaction")

	res, err := stream.Recv()
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "the ordering service returned no status")
	gomega.Expect(res.GetStatus()).To(gomega.Equal(cb.Status_SUCCESS),
		"the ordering service rejected the configuration transaction: [%s]", res.GetInfo())
}
