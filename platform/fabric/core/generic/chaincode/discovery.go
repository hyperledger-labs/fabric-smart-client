/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"context"
	"crypto/sha256"
	"slices"
	"strings"
	"sync"
	"time"

	discovery2 "github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/cache"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	DiscoveryCacheTimeout = time.Minute
)

type Discovery struct {
	chaincode *Chaincode

	FilterByMSPIDs      []string
	ImplicitCollections []string
	QueryForPeers       bool

	discoveryResultsCacheLock sync.RWMutex
	discoveryResultsCache     cache.Map[string, discovery.Response]
}

// NewDiscovery create a discovery client that helps to fetch peer information.
//
// Discovery results are cached in a cache owned by chaincode (created once in
// NewChaincode) and shared across every Discovery instance created for that
// chaincode, so its lifetime (and background eviction goroutine) is bound to
// the chaincode rather than to this single Discovery instance.
func NewDiscovery(chaincode *Chaincode) *Discovery {
	return &Discovery{
		chaincode:             chaincode,
		discoveryResultsCache: chaincode.discoveryResultsCache,
	}
}

func (d *Discovery) Call() ([]driver.DiscoveredPeer, error) {
	if d.QueryForPeers {
		return d.GetPeers()
	}
	return d.GetEndorsers()
}

func (d *Discovery) GetEndorsers() ([]driver.DiscoveredPeer, error) {
	response, err := d.Response()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get discovery response")
	}

	// extract endorsers
	cr := response.ForChannel(d.chaincode.ChannelID)
	var endorsers discovery.Endorsers
	switch {
	case len(d.ImplicitCollections) > 0:
		for _, collection := range d.ImplicitCollections {
			discoveredEndorsers, err := cr.Endorsers(
				ccCall(d.chaincode.name),
				&byMSPIDs{mspIDs: []string{collection}},
			)
			if err != nil {
				return nil, errors.WithMessage(err, "failed to get endorsers")
			}
			endorsers = append(endorsers, discoveredEndorsers...)
		}
	default:
		endorsers, err = cr.Endorsers(
			ccCall(d.chaincode.name),
			&byMSPIDs{mspIDs: d.FilterByMSPIDs},
		)
	}
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting endorsers for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}

	return d.toDiscoveredPeers(endorsers)
}

func (d *Discovery) GetPeers() ([]driver.DiscoveredPeer, error) {
	response, err := d.Response()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get discovery response")
	}

	// extract peers
	cr := response.ForChannel(d.chaincode.ChannelID)
	var peers []*discovery.Peer
	peers, err = cr.Peers(ccCall(d.chaincode.name)...)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting peers for [%s:%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}

	// filter
	switch {
	case len(d.ImplicitCollections) > 0:
		for _, collection := range d.ImplicitCollections {
			peers = (&byMSPIDs{mspIDs: []string{collection}}).Filter(peers)
		}
	default:
		peers = (&byMSPIDs{mspIDs: d.FilterByMSPIDs}).Filter(peers)
	}

	return d.toDiscoveredPeers(peers)
}

func (d *Discovery) Response() (discovery.Response, error) {
	var sb strings.Builder
	sb.WriteString(d.chaincode.NetworkID)
	sb.WriteString(d.chaincode.ChannelID)
	sb.WriteString(d.chaincode.name)
	for _, mspiD := range d.FilterByMSPIDs {
		sb.WriteString(mspiD)
	}
	if d.QueryForPeers {
		sb.WriteString("QueryForPeers")
	}
	key := sb.String()

	// Do we have a response already?
	d.discoveryResultsCacheLock.RLock()
	resp, ok := d.discoveryResultsCache.Get(key)
	d.discoveryResultsCacheLock.RUnlock()
	if ok {
		return resp, nil
	}

	d.discoveryResultsCacheLock.Lock()
	defer d.discoveryResultsCacheLock.Unlock()

	if resp, ok := d.discoveryResultsCache.Get(key); ok {
		return resp, nil
	}

	// fetch the response
	var response discovery.Response
	var err error
	if d.QueryForPeers {
		response, err = d.queryPeers()
	} else {
		response, err = d.queryEndorsers()
	}
	if err != nil {
		return nil, errors.WithMessage(err, "failed to send discovery request")
	}

	// cache response
	d.discoveryResultsCache.Put(key, response)

	// done
	return response, nil
}

func (d *Discovery) WithForQuery() driver.ChaincodeDiscover {
	d.QueryForPeers = true
	return d
}

func (d *Discovery) WithFilterByMSPIDs(mspIDs ...string) driver.ChaincodeDiscover {
	d.FilterByMSPIDs = mspIDs
	return d
}

func (d *Discovery) WithImplicitCollections(mspIDs ...string) driver.ChaincodeDiscover {
	d.ImplicitCollections = mspIDs
	return d
}

func (d *Discovery) queryPeers() (discovery.Response, error) {
	// New discovery request for:
	// - peers and
	// - config,
	req := discovery.NewRequest().OfChannel(d.chaincode.ChannelID).AddPeersQuery(
		&peer.ChaincodeCall{Name: d.chaincode.name},
	)
	req = req.AddConfigQuery()
	return d.query(req)
}

func (d *Discovery) queryEndorsers() (discovery.Response, error) {
	// New discovery request for:
	// - endorsers and
	// - config,
	req, err := discovery.NewRequest().OfChannel(d.chaincode.ChannelID).AddEndorsersQuery(
		&peer.ChaincodeInterest{Chaincodes: []*peer.ChaincodeCall{{Name: d.chaincode.name}}},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating request")
	}
	req = req.AddConfigQuery()
	return d.query(req)
}

func (d *Discovery) query(req *discovery.Request) (discovery.Response, error) {
	var peerClients []peer2.Client
	defer func() {
		for _, pCli := range peerClients {
			pCli.Close()
		}
	}()
	pc, err := d.chaincode.Services.NewPeerClient(*d.chaincode.ConfigService.PickPeer(driver.PeerForDiscovery))
	if err != nil {
		return nil, err
	}
	peerClients = append(peerClients, pc)

	signer := d.chaincode.LocalMembership.DefaultSigningIdentity()
	signerRaw, err := signer.Serialize()
	if err != nil {
		return nil, err
	}
	var ClientTLSCertHash []byte
	if len(pc.Certificate().Certificate) != 0 {
		h := sha256.Sum256(pc.Certificate().Certificate[0])
		ClientTLSCertHash = h[:]
	}
	req.Authentication = &discovery2.AuthInfo{
		ClientIdentity:    signerRaw,
		ClientTlsCertHash: ClientTLSCertHash,
	}
	timeout, cancel := context.WithTimeout(context.Background(), d.chaincode.ChannelConfig.DiscoveryTimeout())
	defer cancel()
	cl, err := pc.DiscoveryClient()
	if err != nil {
		return nil, errors.Wrap(err, "failed creating discovery client")
	}
	response, err := cl.Send(timeout, req, &discovery2.AuthInfo{
		ClientIdentity:    signerRaw,
		ClientTlsCertHash: ClientTLSCertHash,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed requesting endorsers")
	}

	return response, nil
}

// toDiscoveredPeers turns the peers reported by a discovery response into
// [driver.DiscoveredPeer] values, dropping any the channel does not recognize.
//
// A discovery response is supplied by whichever peer answered the query and is
// not independently verified: the envelope signature checks made while parsing
// it prove only that each envelope was signed by the key in the identity shipped
// alongside it, which a malicious responder satisfies with an identity no CA
// ever issued. Every peer is therefore checked against the channel's own
// membership, obtained from [MSPProvider].
//
// That makes the channel configuration the trust anchor, and it is not itself
// cryptographically verified here: it is trusted because it was fetched over TLS
// from a locally configured peer rather than from the discovery responder.
//
// A peer the channel does not recognize is dropped rather than failing the call,
// so one rogue entry in a response cannot deny service. Dropping peers can leave
// a set that no longer satisfies the chaincode's endorsement policy; where that
// is decidable from the request — the per-organization sets of
// [Discovery.WithImplicitCollections] — an emptied organization is reported
// here instead.
//
// It reports [driver.ErrNotInitialized] or [driver.ErrConfigRejected] unchanged
// if the channel configuration is unavailable, since that is not a verdict on
// any peer. A call that has to wait for a configuration pays the provider's wait
// budget once, on top of the discovery query's own timeout.
func (d *Discovery) toDiscoveredPeers(endorsers []*discovery.Peer) ([]driver.DiscoveredPeer, error) {
	// Obtained once for the whole call so that every peer is validated against
	// the same membership, rather than against whatever each iteration happens
	// to resolve.
	mspManager := d.chaincode.MSPProvider.MSPManager()

	var discoveredEndorsers []driver.DiscoveredPeer
	rejectedByMSPID := make(map[string]int)
	for _, peer := range endorsers {
		// extract peer info
		if peer.AliveMessage == nil {
			continue
		}
		aliveMsg := peer.AliveMessage.GetAliveMsg()
		if aliveMsg == nil {
			continue
		}
		member := aliveMsg.Membership
		if member == nil {
			logger.Debugf("no membership info in alive message for peer [%s:%s]", peer.MSPID, view.Identity(peer.Identity).String())
			continue
		}

		tlsRootCerts, err := d.validatePeer(mspManager, peer)
		if err != nil {
			// A configuration that has not arrived, or was refused, is not a
			// verdict on this peer: report it rather than reporting the peer as
			// untrusted.
			if errors.Is(err, driver.ErrNotInitialized) || errors.Is(err, driver.ErrConfigRejected) {
				return nil, errors.WithMessagef(err, "cannot validate discovered peers for [%s:%s]", d.chaincode.NetworkID, d.chaincode.ChannelID)
			}
			rejectedByMSPID[peer.MSPID]++
			logger.Warnf("dropping discovered peer [%s:%s] at [%s]: %v", peer.MSPID, view.Identity(peer.Identity).String(), member.Endpoint, err)
			continue
		}

		discoveredEndorsers = append(discoveredEndorsers, driver.DiscoveredPeer{
			Identity:     peer.Identity,
			MSPID:        peer.MSPID,
			Endpoint:     member.Endpoint,
			TLSRootCerts: tlsRootCerts,
		})
	}

	var rejected int
	for _, n := range rejectedByMSPID {
		rejected += n
	}
	if len(discoveredEndorsers) == 0 && rejected > 0 {
		return nil, errors.Errorf("all %d discovered peers for [%s:%s:%s] failed MSP validation", rejected, d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}
	if err := d.checkRequiredMSPIDsSurvived(discoveredEndorsers, rejectedByMSPID); err != nil {
		return nil, err
	}

	return discoveredEndorsers, nil
}

// checkRequiredMSPIDsSurvived reports an error if validation emptied an
// organization the request cannot be satisfied without.
//
// Dropping an unrecognized peer is normally preferable to failing, but that
// rests on the surviving peers still being able to satisfy the request. For the
// implicit collections of [Discovery.WithImplicitCollections] that does not
// hold: endorsers are gathered per organization precisely because every one of
// them has to endorse, so an organization left with no peers makes the whole set
// useless.
// Reporting it here names the organization and the validation failure, where
// returning the truncated set would surface later as an opaque endorsement or
// collection-policy error.
//
// [Discovery.WithFilterByMSPIDs] is deliberately not checked: it narrows the
// organizations a caller will accept an endorser from, and any one of them
// satisfies it.
func (d *Discovery) checkRequiredMSPIDsSurvived(kept []driver.DiscoveredPeer, rejectedByMSPID map[string]int) error {
	for _, mspID := range d.ImplicitCollections {
		rejected := rejectedByMSPID[mspID]
		if rejected == 0 {
			// Nothing was dropped for this organization, so an empty result is
			// what discovery itself reported and not something validation did.
			continue
		}
		if slices.ContainsFunc(kept, func(p driver.DiscoveredPeer) bool { return p.MSPID == mspID }) {
			continue
		}

		return errors.Errorf("all %d discovered peers of MSP [%s] for [%s:%s:%s] failed MSP validation, and its implicit collection cannot be endorsed without one", rejected, mspID, d.chaincode.NetworkID, d.chaincode.ChannelID, d.chaincode.name)
	}

	return nil
}

// validatePeer checks a discovered peer's identity against the channel's MSPs
// and returns the TLS certificates to authenticate it with, both taken from the
// channel configuration rather than from the response the peer came in.
func (d *Discovery) validatePeer(mspManager driver.MSPManager, peer *discovery.Peer) ([][]byte, error) {
	identity, err := mspManager.DeserializeIdentity(peer.Identity)
	if err != nil {
		return nil, errors.WithMessage(err, "identity is not one the channel recognizes")
	}
	if err := identity.Validate(); err != nil {
		return nil, errors.WithMessage(err, "identity failed MSP validation")
	}
	// The MSP ID the response claimed decides which organization's TLS roots
	// authenticate the connection, so an identity validated under one MSP must
	// not be usable as a peer of another.
	if actual := identity.GetMSPIdentifier(); actual != peer.MSPID {
		return nil, errors.Errorf("identity belongs to MSP [%s] but was reported under [%s]", actual, peer.MSPID)
	}

	tlsRootCerts, err := d.chaincode.MSPProvider.TLSRootCertsByMSPID(peer.MSPID)
	if err != nil {
		return nil, errors.WithMessagef(err, "no trusted TLS roots for MSP [%s]", peer.MSPID)
	}
	return tlsRootCerts, nil
}

func (d *Discovery) ChaincodeVersion() (string, error) {
	response, err := d.Response()
	if err != nil {
		return "", errors.Wrapf(err, "unable to discover channel information for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	endorsers, err := response.ForChannel(d.chaincode.ChannelID).Endorsers([]*peer.ChaincodeCall{{
		Name: d.chaincode.name,
	}}, &noFilter{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to get endorsers for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	if len(endorsers) == 0 {
		return "", errors.Errorf("no endorsers found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	stateInfoMessage := endorsers[0].StateInfoMessage
	if stateInfoMessage == nil {
		return "", errors.Errorf("no state info message found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	stateInfo := stateInfoMessage.GetStateInfo()
	if stateInfo == nil {
		return "", errors.Errorf("no state info found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	properties := stateInfo.GetProperties()
	if properties == nil {
		return "", errors.Errorf("no properties found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	chaincodes := properties.Chaincodes
	if len(chaincodes) == 0 {
		return "", errors.Errorf("no chaincode info found for chaincode [%s] on channel [%s]", d.chaincode.name, d.chaincode.ChannelID)
	}
	for _, chaincode := range chaincodes {
		if chaincode.Name == d.chaincode.name {
			return chaincode.Version, nil
		}
	}
	return "", errors.Errorf("chaincode [%s] not found", d.chaincode.name)
}

func ccCall(ccNames ...string) []*peer.ChaincodeCall {
	var call []*peer.ChaincodeCall
	for _, ccName := range ccNames {
		call = append(call, &peer.ChaincodeCall{
			Name: ccName,
		})
	}
	return call
}

type byMSPIDs struct {
	mspIDs []string
}

func (f *byMSPIDs) Filter(endorsers discovery.Endorsers) discovery.Endorsers {
	if len(f.mspIDs) == 0 {
		return endorsers
	}

	var filteredEndorsers discovery.Endorsers
	for _, endorser := range endorsers {
		endorserMSPID := endorser.MSPID
		found := slices.Contains(f.mspIDs, endorserMSPID)
		if !found {
			continue
		}
		filteredEndorsers = append(filteredEndorsers, endorser)
	}
	return filteredEndorsers
}

type noFilter struct{}

func (f *noFilter) Filter(endorsers discovery.Endorsers) discovery.Endorsers {
	return endorsers
}
